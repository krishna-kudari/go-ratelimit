(function () {
  "use strict";

  var C = {
    volt: "#DCFF1E",
    voltDim: "rgba(220, 255, 30, 0.25)",
    red: "#FF4438",
    redDim: "rgba(255, 68, 56, 0.25)",
    skyBlue: "#80DBFF",
    skyBlueDim: "rgba(128, 219, 255, 0.2)",
    violet: "#C795E3",
    violetDim: "rgba(199, 149, 227, 0.25)",
    border: "#2D4754",
    surface: "#163341",
    midnight: "#091A23",
    textPrimary: "#D9D9D9",
    textSecondary: "#B9C2C6",
    textMuted: "#8A99A0",
    textDim: "#5C707A",
  };

  var current = null;
  var animFrame = null;
  var lastTime = 0;

  function h(tag, className, props) {
    var e = document.createElement(tag);
    if (className) e.className = className;
    if (props) {
      for (var k in props) {
        var v = props[k];
        if (k === "text") e.textContent = v;
        else if (k === "html") e.innerHTML = v;
        else if (k === "style" && typeof v === "object") Object.assign(e.style, v);
        else e.setAttribute(k, String(v));
      }
    }
    return e;
  }

  function colorForPct(pct) {
    if (pct > 0.6) return C.volt;
    if (pct > 0.25) return C.violet;
    return C.red;
  }

  function flashVizPanel() {
    var vizBox = document.querySelector("#algorithm-detail .viz-panel");
    if (vizBox) {
      vizBox.classList.remove("flash-denied");
      void vizBox.offsetWidth;
      vizBox.classList.add("flash-denied");
    }
  }

  var renderers = {};

  // ========== Fixed Window ==========
  renderers["fixed-window"] = {
    create: function (container, config) {
      var max = config.maxRequests || 10;
      var winSec = config.windowSeconds || 10;

      var statusLine = h("div", "viz-status", { style: { color: C.textSecondary }, text: "Window: " + winSec.toFixed(1) + "s remaining" });
      container.appendChild(statusLine);

      var grid = h("div", "slot-grid");
      var slots = [];
      var slotW = Math.max(16, Math.min(32, Math.floor(380 / max) - 4));
      for (var i = 0; i < max; i++) {
        var s = h("div", "slot", { style: { width: slotW + "px", height: "56px", border: "2px solid " + C.border, background: "transparent" } });
        grid.appendChild(s);
        slots.push(s);
      }
      container.appendChild(grid);

      var barBg = h("div", "progress-bar", { style: { height: "6px", background: C.surface, marginBottom: "12px" } });
      var barFill = h("div", "progress-fill", { style: { height: "6px", width: "100%", background: C.skyBlue, transition: "width 0.15s linear" } });
      barBg.appendChild(barFill);
      container.appendChild(barBg);

      var countLine = h("div", "viz-count", { style: { color: C.textDim }, text: "0 / " + max + " requests used" });
      container.appendChild(countLine);

      return { slots: slots, barFill: barFill, statusLine: statusLine, countLine: countLine, local: { used: 0, timeLeft: winSec, active: false, windowSeconds: winSec } };
    },
    update: function (els, result) {
      var used = result.limit - result.remaining;
      els.local.used = used;
      els.local.active = true;
      if (result.retryAfter !== null) els.local.timeLeft = result.retryAfter;

      els.slots.forEach(function (s, i) {
        if (i < used) {
          var color = colorForPct(1 - i / result.limit);
          s.style.background = color;
          s.style.borderColor = color;
        } else {
          s.style.background = "transparent";
          s.style.borderColor = C.border;
        }
      });

      if (used > 0 && used <= els.slots.length) {
        var slot = els.slots[used - 1];
        slot.classList.remove("slot-pop");
        void slot.offsetWidth;
        slot.classList.add("slot-pop");
      }

      if (!result.allowed) flashVizPanel();
      els.countLine.textContent = used + " / " + result.limit + " requests used";
    },
    tick: function (els, config, dt) {
      if (!els.local.active) return;
      var winSec = config.windowSeconds || 10;
      els.local.timeLeft -= dt;
      if (els.local.timeLeft <= 0) {
        els.local.timeLeft = winSec;
        els.local.used = 0;
        els.local.active = false;
        els.slots.forEach(function (s) { s.style.background = "transparent"; s.style.borderColor = C.border; });
        els.countLine.textContent = "0 / " + (config.maxRequests || 10) + " requests used";
      }
      var pct = Math.max(0, els.local.timeLeft / winSec);
      els.barFill.style.width = (pct * 100) + "%";
      els.statusLine.textContent = "Window: " + Math.max(0, els.local.timeLeft).toFixed(1) + "s remaining";
    }
  };

  // ========== Sliding Window Log ==========
  renderers["sliding-window-log"] = {
    create: function (container, config) {
      var max = config.maxRequests || 10;
      var winSec = config.windowSeconds || 10;

      var statusLine = h("div", "viz-status", { style: { color: C.textSecondary }, text: "0 / " + max + " requests in window" });
      container.appendChild(statusLine);

      var timeline = h("div", "timeline", { style: { height: "90px", border: "1px solid " + C.skyBlue + "50", borderRadius: "8px", background: C.skyBlue + "08", overflow: "hidden" } });
      var midline = h("div", "timeline-line", { style: { top: "50%", height: "2px", background: C.border, transform: "translateY(-50%)" } });
      timeline.appendChild(midline);
      var dotsWrap = h("div", "dots-layer");
      timeline.appendChild(dotsWrap);
      container.appendChild(timeline);

      var windowLabel = h("div", "viz-label", { style: { color: C.textDim }, text: "\u2190 " + winSec + "s window \u2192" });
      container.appendChild(windowLabel);

      var countLine = h("div", "viz-count", { style: { color: C.textDim }, text: "0 / " + max + " requests in window" });
      container.appendChild(countLine);

      return { timeline: timeline, dotsWrap: dotsWrap, statusLine: statusLine, countLine: countLine, local: { dots: [], windowSeconds: winSec, maxRequests: max } };
    },
    update: function (els, result) {
      var now = Date.now();
      var nearbyCount = els.local.dots.filter(function (d) { return Math.abs(now - d.ts) < 10; }).length;

      var dot = h("div", "dot dot-appear" + (result.allowed ? " allowed" : ""), {
        style: { width: (10 + nearbyCount * 2) + "px", height: (10 + nearbyCount * 2) + "px", top: "50%", right: "10px", background: result.allowed ? C.volt : C.red, boxShadow: "0 0 6px " + (result.allowed ? C.voltDim : C.redDim), transform: "translateY(-50%) scale(0)" }
      });
      els.dotsWrap.appendChild(dot);
      els.local.dots.push({ ts: now, el: dot });

      if (!result.allowed) flashVizPanel();
      var count = result.limit - result.remaining;
      els.statusLine.textContent = count + " / " + result.limit + " requests in window";
      els.countLine.textContent = count + " / " + result.limit + " requests in window";
    },
    tick: function (els, config, dt) {
      var now = Date.now();
      var winMs = (config.windowSeconds || 10) * 1000;
      var tw = els.timeline.offsetWidth || 400;
      var count = 0;

      els.local.dots = els.local.dots.filter(function (d) {
        var age = now - d.ts;
        if (age > winMs) {
          d.el.style.opacity = "0";
          d.el.style.transition = "opacity 0.4s";
          setTimeout(function () { d.el.remove(); }, 400);
          return false;
        }
        var pct = Math.max(0, age / winMs);
        d.el.style.right = (10 + pct * (tw - 30)) + "px";
        if (d.el.classList.contains("allowed")) count++;
        return true;
      });

      els.statusLine.textContent = els.statusLine.textContent.replace(/^\d+/, count);
      els.countLine.textContent = els.countLine.textContent.replace(/^\d+/, count);
    }
  };

  // ========== Sliding Window Counter ==========
  renderers["sliding-window-counter"] = {
    create: function (container, config) {
      var max = config.maxRequests || 10;
      var winSec = config.windowSeconds || 10;

      var statusLine = h("div", "viz-status", { style: { color: C.textSecondary }, text: "Weight: 1.00 \u2014 Effective: 0 / " + max });
      container.appendChild(statusLine);

      var frame = h("div", "timeline", { style: { height: "90px", border: "1px solid " + C.skyBlue + "50", borderRadius: "8px", background: C.skyBlue + "08", overflow: "hidden" } });
      var track = h("div", "track", { style: { width: "200%", left: "0", transform: "translateX(0%)", transition: "transform 0.15s linear" } });

      function makeWindow(label, bg, labelColor) {
        var win = h("div", "window-half", { style: { width: "50%", background: bg } });
        var lbl = h("div", "window-label", { style: { color: labelColor }, text: label });
        win.appendChild(lbl);
        var line = h("div", "window-line", { style: { top: "50%", height: "2px", background: C.border, transform: "translateY(-50%)" } });
        win.appendChild(line);
        var dots = h("div", "dots-layer");
        win.appendChild(dots);
        return { win: win, lbl: lbl, dots: dots };
      }

      var prev = makeWindow("Previous Window", C.violet + "0a", C.violet + "90");
      prev.win.style.left = "0";
      track.appendChild(prev.win);

      var curr = makeWindow("Current Window", C.volt + "08", C.volt + "90");
      curr.win.style.left = "50%";
      track.appendChild(curr.win);

      var boundary = h("div", "track-boundary", { style: { left: "50%", width: "0px", borderLeft: "1px dashed " + C.textDim + "60" } });
      track.appendChild(boundary);
      frame.appendChild(track);
      container.appendChild(frame);

      var windowLabel = h("div", "viz-label", { style: { color: C.skyBlue }, text: "\u2190 " + winSec + "s sliding window \u2192" });
      container.appendChild(windowLabel);

      var formula = h("div", "viz-formula", { style: { color: C.textDim }, text: "0 \u00d7 1.00 + 0 = 0 effective" });
      container.appendChild(formula);

      return { track: track, prev: prev, curr: curr, formula: formula, statusLine: statusLine, local: { prevCount: 0, currCount: 0, prevDots: [], currDots: [], weight: 1, windowStart: Date.now(), windowSeconds: winSec, maxRequests: max, lastWindowNum: 0 } };
    },
    update: function (els, result) {
      var now = Date.now();
      els.local.currCount++;

      var winSec = els.local.windowSeconds;
      var rawElapsed = (now - els.local.windowStart) / 1000;
      var nearbyCount = els.local.currDots.filter(function (d) { return Math.abs(now - d.ts) < 50; }).length;
      var adjusted = rawElapsed + nearbyCount * 0.018;
      var progress = (adjusted % winSec) / winSec;
      var xPct = 4 + progress * 92;

      var lanes = [-28, -14, 0, 14, 28];
      var yOffset = lanes[nearbyCount % lanes.length];

      var dot = h("div", "dot dot-appear" + (result.allowed ? " allowed" : ""), {
        style: { width: (10 + nearbyCount * 2) + "px", height: (10 + nearbyCount * 2) + "px", left: xPct + "%", top: "50%", background: result.allowed ? C.volt : C.red, boxShadow: "0 0 6px " + (result.allowed ? C.voltDim : C.redDim), transform: "translateY(-50%) scale(0)" }
      });
      els.curr.dots.appendChild(dot);
      els.local.currDots.push({ el: dot, ts: now });

      var max = result.limit;
      var effective = max - result.remaining;
      els.curr.lbl.textContent = "Current Window (" + els.local.currCount + ")";
      els.prev.lbl.textContent = "Previous Window (" + els.local.prevCount + ")";

      var weighted = els.local.prevCount * els.local.weight;
      els.formula.textContent = els.local.prevCount + " \u00d7 " + els.local.weight.toFixed(2) + " + " + els.local.currCount + " = " + (weighted + els.local.currCount).toFixed(1);
      els.statusLine.textContent = "Weight: " + els.local.weight.toFixed(2) + " \u2014 Effective: " + effective + " / " + max;

      if (!result.allowed) flashVizPanel();
    },
    tick: function (els, config, dt) {
      var winSec = config.windowSeconds || 10;
      var elapsed = (Date.now() - els.local.windowStart) / 1000;
      var currentWindowNum = Math.floor(elapsed / winSec);
      var windowProgress = (elapsed % winSec) / winSec;

      if (currentWindowNum > els.local.lastWindowNum) {
        els.local.lastWindowNum = currentWindowNum;
        els.local.prevCount = els.local.currCount;
        els.local.currCount = 0;

        els.prev.dots.innerHTML = "";
        els.local.prevDots.forEach(function (d) { d.el.remove(); });
        els.local.prevDots = els.local.currDots;
        els.local.currDots = [];
        els.local.prevDots.forEach(function (d) { els.prev.dots.appendChild(d.el); });
        els.curr.dots.innerHTML = "";

        els.prev.lbl.textContent = "Previous Window (" + els.local.prevCount + ")";
        els.curr.lbl.textContent = "Current Window (0)";

        els.track.style.transition = "none";
        els.track.style.transform = "translateX(0%)";
        requestAnimationFrame(function () { els.track.style.transition = "transform 0.15s linear"; });
      }

      els.track.style.transform = "translateX(" + (-windowProgress * 50) + "%)";
      els.local.weight = +(1 - windowProgress).toFixed(3);
      els.prev.dots.style.opacity = String(0.3 + els.local.weight * 0.7);

      var weighted = els.local.prevCount * els.local.weight;
      var effective = weighted + els.local.currCount;
      var max = config.maxRequests || 10;
      els.formula.textContent = els.local.prevCount + " \u00d7 " + els.local.weight.toFixed(2) + " + " + els.local.currCount + " = " + effective.toFixed(1);
      els.statusLine.textContent = "Weight: " + els.local.weight.toFixed(2) + " \u2014 Effective: " + effective.toFixed(1) + " / " + max;
    }
  };

  // ========== Token Bucket ==========
  renderers["token-bucket"] = {
    create: function (container, config) {
      var max = config.maxTokens || 10;
      var rate = config.refillRate || 1;

      var statusLine = h("div", "viz-status", { style: { color: C.textSecondary }, html: '<span style="color:' + C.volt + '">\u25bc</span> Refilling at ' + rate + " token" + (rate !== 1 ? "s" : "") + "/s" });
      container.appendChild(statusLine);

      var bucketWrap = h("div", "bucket-wrap");
      var bucket = h("div", "bucket", { style: { width: "260px", height: "160px", borderLeft: "3px solid " + C.border, borderRight: "3px solid " + C.border, borderBottom: "3px solid " + C.border, borderRadius: "0 0 14px 14px" } });
      var tokenGrid = h("div", "token-grid");
      var tokens = [];
      for (var i = 0; i < max; i++) {
        var t = h("div", "token", { style: { width: "22px", height: "22px", background: C.volt, boxShadow: "0 0 6px " + C.voltDim, transition: "transform 0.3s, opacity 0.3s" } });
        tokenGrid.appendChild(t);
        tokens.push(t);
      }
      bucket.appendChild(tokenGrid);
      bucketWrap.appendChild(bucket);
      container.appendChild(bucketWrap);

      var countLine = h("div", "viz-count", { style: { color: C.textDim }, text: max + " / " + max + " tokens" });
      container.appendChild(countLine);

      return { tokens: tokens, countLine: countLine, statusLine: statusLine, local: { count: max, maxTokens: max, refillRate: rate, fractional: 0 } };
    },
    update: function (els, result) {
      els.local.count = result.remaining;
      els.local.fractional = 0;

      els.tokens.forEach(function (t, i) {
        if (i < result.remaining) {
          t.style.background = C.volt;
          t.style.boxShadow = "0 0 6px " + C.voltDim;
          t.style.transform = "scale(1)";
          t.style.opacity = "1";
          t.classList.remove("token-consume");
        } else {
          t.classList.remove("token-consume", "token-appear");
          void t.offsetWidth;
          if (i === result.remaining) {
            t.classList.add("token-consume");
          } else {
            t.style.transform = "scale(0)";
            t.style.opacity = "0.15";
            t.style.background = C.border;
            t.style.boxShadow = "none";
          }
        }
      });

      if (!result.allowed) flashVizPanel();
      els.countLine.textContent = result.remaining + " / " + result.limit + " tokens";
    },
    tick: function (els, config, dt) {
      var max = config.maxTokens || 10;
      var rate = config.refillRate || 1;
      if (els.local.count >= max) return;

      els.local.fractional += rate * dt;
      if (els.local.fractional >= 1) {
        var toAdd = Math.floor(els.local.fractional);
        els.local.fractional -= toAdd;
        var newCount = Math.min(max, els.local.count + toAdd);

        for (var i = els.local.count; i < newCount; i++) {
          if (els.tokens[i]) {
            var t = els.tokens[i];
            t.classList.remove("token-consume");
            t.style.background = C.volt;
            t.style.boxShadow = "0 0 6px " + C.voltDim;
            t.style.transform = "scale(1)";
            t.style.opacity = "1";
            t.classList.add("token-appear");
          }
        }
        els.local.count = newCount;
        els.countLine.textContent = newCount + " / " + max + " tokens";
      }
    }
  };

  // ========== Leaky Bucket ==========
  renderers["leaky-bucket"] = {
    create: function (container, config) {
      var cap = config.capacity || 10;
      var rate = config.leakRate || 1;
      var mode = config.mode || "policing";
      var isShaping = mode === "shaping";

      var statusLine = h("div", "viz-status", {
        style: { color: C.textSecondary },
        html: isShaping
          ? '<span style="color:' + C.violet + '">\u29D7</span> Shaping \u2014 requests queued'
          : '<span style="color:' + C.skyBlue + '">\u25bc</span> Policing \u2014 excess dropped'
      });
      container.appendChild(statusLine);

      var bucketWrap = h("div", "bucket-wrap-sm");
      var bucket = h("div", "bucket", { style: { width: "260px", height: "150px", borderLeft: "3px solid " + C.border, borderRight: "3px solid " + C.border, borderBottom: "3px solid " + C.border, borderRadius: "0 0 14px 14px" } });
      var water = h("div", "water-surface water-fill" + (isShaping ? " shaping" : ""), { style: { height: "0%" } });
      bucket.appendChild(water);

      for (var i = 1; i <= 4; i++) {
        var marker = h("div", "bucket-marker", { style: { bottom: (i / 5 * 100) + "%", height: "1px", background: C.border + "80" } });
        bucket.appendChild(marker);
      }

      bucketWrap.appendChild(bucket);
      container.appendChild(bucketWrap);

      var dripRow = h("div", "drip-row");
      var dripDot = h("div", "drip drip-dot", { style: { width: "6px", height: "6px", background: isShaping ? C.violet : C.skyBlue, opacity: "0" } });
      dripRow.appendChild(dripDot);
      container.appendChild(dripRow);

      var drainLabel = h("div", "viz-label-drain", { style: { color: C.textDim }, text: isShaping ? "Processing at " + rate + " req/s" : "Draining at " + rate + " req/s" });
      container.appendChild(drainLabel);

      var countLine = h("div", "viz-count", { style: { color: C.textDim }, text: isShaping ? "Queue: 0 / " + cap : "Level: 0 / " + cap });
      container.appendChild(countLine);

      return { water: water, dripDot: dripDot, countLine: countLine, drainLabel: drainLabel, statusLine: statusLine, local: { level: 0, capacity: cap, leakRate: rate, mode: mode } };
    },
    update: function (els, result) {
      var level = result.limit - result.remaining;
      els.local.level = level;
      var isShaping = els.local.mode === "shaping";

      var pct = (level / els.local.capacity) * 100;
      els.water.style.height = pct + "%";

      if (isShaping) {
        els.water.classList.remove("danger");
      } else if (pct > 80) {
        els.water.classList.add("danger");
      } else {
        els.water.classList.remove("danger");
      }

      if (level > 0) els.dripDot.style.opacity = "0.8";
      if (!result.allowed) flashVizPanel();

      var label = isShaping ? "Queue" : "Level";
      els.countLine.textContent = label + ": " + level + " / " + result.limit;

      if (isShaping && result.delay != null && result.delay > 0) {
        els.statusLine.innerHTML = '<span style="color:' + C.violet + '">\u29D7</span> Shaping \u2014 delay: ' + result.delay.toFixed(1) + "s";
      }
    },
    tick: function (els, config, dt) {
      var rate = config.leakRate || 1;
      var cap = config.capacity || 10;
      var isShaping = els.local.mode === "shaping";

      if (els.local.level > 0) {
        els.local.level = Math.max(0, els.local.level - rate * dt);
        var pct = (els.local.level / cap) * 100;
        els.water.style.height = pct + "%";

        if (!isShaping) {
          if (pct > 80) els.water.classList.add("danger");
          else els.water.classList.remove("danger");
        }

        var display = Math.ceil(els.local.level);
        var label = isShaping ? "Queue" : "Level";
        els.countLine.textContent = label + ": " + display + " / " + cap;

        if (els.local.level <= 0) {
          els.dripDot.style.opacity = "0";
          if (isShaping) {
            els.statusLine.innerHTML = '<span style="color:' + C.violet + '">\u29D7</span> Shaping \u2014 requests queued';
          }
        }
      }
    }
  };

  // ========== GCRA ==========
  renderers["gcra"] = {
    create: function (container, config) {
      var burst = config.burst || 10;
      var rate = config.rate || 5;

      var statusLine = h("div", "viz-status", {
        style: { color: C.textSecondary },
        html: '<span style="color:' + C.skyBlue + '">\u25bc</span> Rate: ' + rate + ' req/s \u2014 Burst: ' + burst
      });
      container.appendChild(statusLine);

      var bucketWrap = h("div", "bucket-wrap");
      var bucket = h("div", "bucket", { style: { width: "260px", height: "160px", borderLeft: "3px solid " + C.border, borderRight: "3px solid " + C.border, borderBottom: "3px solid " + C.border, borderRadius: "0 0 14px 14px" } });
      var tokenGrid = h("div", "token-grid");
      var tokens = [];
      for (var i = 0; i < burst; i++) {
        var t = h("div", "token", { style: { width: "22px", height: "22px", background: C.skyBlue, boxShadow: "0 0 6px " + C.skyBlueDim, transition: "transform 0.3s, opacity 0.3s" } });
        tokenGrid.appendChild(t);
        tokens.push(t);
      }
      bucket.appendChild(tokenGrid);
      bucketWrap.appendChild(bucket);
      container.appendChild(bucketWrap);

      var intervalLabel = h("div", "viz-label", {
        style: { color: C.textDim },
        text: "Emission interval: " + (1000 / rate).toFixed(0) + "ms"
      });
      container.appendChild(intervalLabel);

      var countLine = h("div", "viz-count", { style: { color: C.textDim }, text: burst + " / " + burst + " burst remaining" });
      container.appendChild(countLine);

      return { tokens: tokens, countLine: countLine, statusLine: statusLine, intervalLabel: intervalLabel, local: { count: burst, burst: burst, rate: rate, fractional: 0 } };
    },
    update: function (els, result) {
      els.local.count = result.remaining;
      els.local.fractional = 0;

      els.tokens.forEach(function (t, i) {
        if (i < result.remaining) {
          t.style.background = C.skyBlue;
          t.style.boxShadow = "0 0 6px " + C.skyBlueDim;
          t.style.transform = "scale(1)";
          t.style.opacity = "1";
          t.classList.remove("token-consume");
        } else {
          t.classList.remove("token-consume", "token-appear");
          void t.offsetWidth;
          if (i === result.remaining) {
            t.classList.add("token-consume");
          } else {
            t.style.transform = "scale(0)";
            t.style.opacity = "0.15";
            t.style.background = C.border;
            t.style.boxShadow = "none";
          }
        }
      });

      if (!result.allowed) flashVizPanel();
      els.countLine.textContent = result.remaining + " / " + result.limit + " burst remaining";
    },
    tick: function (els, config, dt) {
      var burst = config.burst || 10;
      var rate = config.rate || 5;
      if (els.local.count >= burst) return;

      els.local.fractional += rate * dt;
      if (els.local.fractional >= 1) {
        var toAdd = Math.floor(els.local.fractional);
        els.local.fractional -= toAdd;
        var newCount = Math.min(burst, els.local.count + toAdd);

        for (var i = els.local.count; i < newCount; i++) {
          if (els.tokens[i]) {
            var t = els.tokens[i];
            t.classList.remove("token-consume");
            t.style.background = C.skyBlue;
            t.style.boxShadow = "0 0 6px " + C.skyBlueDim;
            t.style.transform = "scale(1)";
            t.style.opacity = "1";
            t.classList.add("token-appear");
          }
        }
        els.local.count = newCount;
        els.countLine.textContent = newCount + " / " + burst + " burst remaining";
      }
    }
  };

  // ---- API helpers ----

  function getConfig() {
    var form = document.getElementById("config-form");
    if (!form) return current ? current.config : {};
    var data = {};
    form.querySelectorAll("input[name]").forEach(function (input) {
      data[input.name] = parseFloat(input.value) || 0;
    });
    form.querySelectorAll("select[name]").forEach(function (select) {
      data[select.name] = select.value;
    });
    return data;
  }

  function addToLog(results) {
    var log = document.getElementById("result-log");
    var empty = document.getElementById("result-log-empty");
    if (!log) return;
    if (empty) empty.style.display = "none";

    results.forEach(function (r) {
      var hasDelay = r.delay != null && r.delay > 0;
      var icon = r.allowed ? (hasDelay ? "\u29D7" : "\u2713") : "\u2717";
      var color = r.allowed ? (hasDelay ? C.violet : C.volt) : C.red;
      var text;
      if (!r.allowed) {
        text = "Denied \u2014 retry after " + (r.retryAfter || 0).toFixed(1) + "s";
      } else if (hasDelay) {
        text = "Queued +" + r.delay.toFixed(1) + "s \u2014 " + r.remaining + "/" + r.limit + " remaining";
      } else {
        text = "Allowed \u2014 " + r.remaining + "/" + r.limit + " remaining";
      }

      var entry = h("div", "log-entry", { style: { borderColor: C.border + "60" } });
      entry.innerHTML = '<span style="color:' + color + '" class="log-icon">' + icon + '</span><span style="color:' + C.textMuted + '" class="log-text">' + text + '</span>';
      log.insertBefore(entry, log.firstChild);
    });

    while (log.children.length > 60) {
      log.removeChild(log.lastChild);
    }
  }

  function sendRequest() {
    if (!current) return;
    var config = getConfig();
    fetch("/api/rate-limit/" + current.algorithm, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ config: config })
    })
      .then(function (resp) { return resp.json(); })
      .then(function (result) {
        renderers[current.algorithm].update(current.elements, result);
        addToLog([result]);
      })
      .catch(function (err) { console.error("Request failed:", err); });
  }

  function sendBurst() {
    if (!current) return;
    var config = getConfig();
    var count = parseInt((document.getElementById("burst-count") || {}).value || "5", 10) || 5;
    fetch("/api/rate-limit/" + current.algorithm + "/burst", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ config: config, count: count })
    })
      .then(function (resp) { return resp.json(); })
      .then(function (results) {
        results.forEach(function (r) { renderers[current.algorithm].update(current.elements, r); });
        addToLog(results);
      })
      .catch(function (err) { console.error("Burst failed:", err); });
  }

  function resetAlgorithm() {
    if (!current) return;
    fetch("/api/rate-limit/reset", { method: "POST" })
      .then(function () {
        var container = document.getElementById("viz-container");
        if (container) {
          container.innerHTML = "";
          current.elements = renderers[current.algorithm].create(container, getConfig());
        }
        var log = document.getElementById("result-log");
        if (log) log.innerHTML = "";
        var empty = document.getElementById("result-log-empty");
        if (empty) empty.style.display = "";
      })
      .catch(function (err) { console.error("Reset failed:", err); });
  }

  // ---- Animation loop ----

  function animate(time) {
    if (!current) return;
    var dt = lastTime ? Math.min((time - lastTime) / 1000, 0.25) : 0;
    lastTime = time;

    var renderer = renderers[current.algorithm];
    if (renderer && renderer.tick && current.elements) {
      renderer.tick(current.elements, getConfig(), dt);
    }

    animFrame = requestAnimationFrame(animate);
  }

  // ---- Lifecycle ----

  function init(algorithm, config) {
    destroy();

    var container = document.getElementById("viz-container");
    if (!container || !renderers[algorithm]) return;

    container.innerHTML = "";
    var elements = renderers[algorithm].create(container, config);

    current = { algorithm: algorithm, config: config, elements: elements };
    lastTime = 0;
    animFrame = requestAnimationFrame(animate);

    document.querySelectorAll("[data-algo-card]").forEach(function (card) {
      card.classList.toggle("active", card.dataset.algoCard === algorithm);
    });

    var form = document.getElementById("config-form");
    if (form) {
      var debounce;
      var onConfigChange = function () {
        clearTimeout(debounce);
        debounce = setTimeout(function () {
          var newConfig = getConfig();
          current.config = newConfig;
          container.innerHTML = "";
          current.elements = renderers[algorithm].create(container, newConfig);
          fetch("/api/rate-limit/reset", { method: "POST" });
          var log = document.getElementById("result-log");
          if (log) log.innerHTML = "";
          var empty = document.getElementById("result-log-empty");
          if (empty) empty.style.display = "";
        }, 600);
      };
      form.addEventListener("input", onConfigChange);
      form.addEventListener("change", onConfigChange);
    }
  }

  function destroy() {
    if (animFrame) cancelAnimationFrame(animFrame);
    animFrame = null;
    current = null;
    lastTime = 0;
  }

  window.App = { init: init, destroy: destroy, sendRequest: sendRequest, sendBurst: sendBurst, resetAlgorithm: resetAlgorithm };
})();
