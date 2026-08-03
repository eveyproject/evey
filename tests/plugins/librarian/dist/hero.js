// Librarian - home.hero module: the global housekeeping tally across every
// library. Self-contained IIFE (the sandbox loads one script); the detection
// block is inlined VERBATIM from scan.js - the test suite fails on drift.
"use strict";
(function () {
  // --- inlined from scan.js (source of truth; kept byte-identical) ---
  // normTitle flattens a title for comparison: lowercase, accents stripped,
  // punctuation collapsed, leading articles dropped (fr/en/de) - "L'Été" and
  // "Ete" must collide, that is the point.
  function normTitle(t) {
    return String(t || "")
      .toLowerCase()
      .normalize("NFD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, " ")
      .replace(/^(le|la|les|l|the|a|an|un|une|der|die|das) /, "")
      .replace(/ +/g, " ")
      .trim();
  }

  var ROMAN = { i: 1, ii: 2, iii: 3, iv: 4, v: 5, vi: 6, vii: 7, viii: 8, ix: 9, x: 10, xi: 11, xii: 12, xiii: 13, xiv: 14, xv: 15 };

  // sagaKey splits "Rocky III" / "Iron Man 2" / "Halloween part 4" into
  // { base, num }. An unnumbered title is episode 1 of its own base - that is
  // what lets "Rocky II without Rocky" surface as a hole. Bounded at 15: past
  // that, a trailing number is more likely a year fragment than an episode.
  function sagaKey(title) {
    var n = normTitle(title);
    var m = /^(.*?)(?: (?:part|partie|chapter|chapitre|episode|vol|volume))? (\d{1,2}|[ivx]{1,4})$/.exec(n);
    if (!m) return { base: n, num: 1 };
    var raw = m[2];
    var num = /^\d+$/.test(raw) ? parseInt(raw, 10) : ROMAN[raw] || 0;
    if (num < 1 || num > 15) return { base: n, num: 1 };
    return { base: m[1].trim(), num: num };
  }

  // findDuplicates: watchable items sharing a normalized title. Same year (or
  // both yearless) = probable duplicate copy; different years = flagged as a
  // remake/edition, worth an eye but softer.
  function findDuplicates(items) {
    var byKey = {};
    for (var i = 0; i < (items || []).length; i++) {
      var it = items[i];
      if (!it || !it.id || !it.title) continue;
      var ty = String(it.type || "");
      if (ty !== "MEDIA_ITEM_TYPE_MOVIE" && ty !== "MEDIA_ITEM_TYPE_SERIES") continue;
      var key = ty + ":" + normTitle(it.title);
      (byKey[key] = byKey[key] || []).push(it);
    }
    var out = [];
    for (var k in byKey) {
      var group = byKey[k];
      if (group.length < 2) continue;
      var years = {};
      for (var j = 0; j < group.length; j++) years[group[j].year || 0] = true;
      out.push({
        title: group[0].title,
        entries: group,
        sameYear: Object.keys(years).length === 1,
      });
    }
    out.sort(function (a, b) { return b.entries.length - a.entries.length; });
    return out;
  }

  // findSagaHoles: numbered groups (at least one entry with num >= 2) whose
  // sequence 1..max misses numbers. Requires >= 2 entries in the group - a
  // lone "Blade II" is a choice, not a hole.
  function findSagaHoles(items) {
    var groups = {};
    for (var i = 0; i < (items || []).length; i++) {
      var it = items[i];
      if (!it || !it.id || !it.title) continue;
      if (String(it.type || "") !== "MEDIA_ITEM_TYPE_MOVIE") continue;
      var key = sagaKey(it.title);
      if (!key.base) continue;
      var g = (groups[key.base] = groups[key.base] || { nums: {}, sample: it });
      g.nums[key.num] = it;
    }
    var out = [];
    for (var base in groups) {
      var nums = Object.keys(groups[base].nums).map(Number);
      if (nums.length < 2) continue;
      var max = Math.max.apply(null, nums);
      if (max < 2) continue;
      var missing = [];
      for (var n = 1; n <= max; n++) if (!groups[base].nums[n]) missing.push(n);
      if (missing.length === 0) continue;
      out.push({ base: base, have: nums.sort(function (a, b) { return a - b; }), missing: missing, sample: groups[base].sample });
    }
    out.sort(function (a, b) { return b.missing.length - a.missing.length; });
    return out;
  }
  // --- end inlined ---

  var STR = {
    fr: {
      title: "Le bibliothécaire",
      dups: function (n) { return n + (n > 1 ? " doublons probables" : " doublon probable"); },
      holes: function (n) { return n + (n > 1 ? " sagas trouées" : " saga trouée"); },
      clean: "Rien à signaler : bibliothèques impeccables.",
      missing: function (nums) { return "il manque " + (nums.length > 1 ? "les épisodes " : "l'épisode ") + nums.join(", "); },
      copies: function (n) { return n + " copies"; },
      remake: "années différentes - remake ou édition ?",
    },
    en: {
      title: "The librarian",
      dups: function (n) { return n + (n > 1 ? " probable duplicates" : " probable duplicate"); },
      holes: function (n) { return n + (n > 1 ? " sagas with holes" : " saga with a hole"); },
      clean: "Nothing to report: libraries are spotless.",
      missing: function (nums) { return "missing " + (nums.length > 1 ? "episodes " : "episode ") + nums.join(", "); },
      copies: function (n) { return n + " copies"; },
      remake: "different years - remake or edition?",
    },
  };

  function h(tag, style, text) {
    var el = document.createElement(tag);
    if (style) el.setAttribute("style", style);
    if (text) el.textContent = text;
    return el;
  }

  function line(icon, text, mediaId) {
    var row = h("div", "display:flex;gap:8px;align-items:center;font-size:13px;padding:2px 0;" + (mediaId ? "cursor:pointer;" : ""));
    row.appendChild(h("span", "font-size:13px;", icon));
    row.appendChild(h("span", "overflow:hidden;text-overflow:ellipsis;white-space:nowrap;", text));
    if (mediaId) row.onclick = function () { window.evey.navigate("/media/" + mediaId); };
    return row;
  }

  window.evey.onInit(function (init) {
    var lang = String(init.locale || "en").slice(0, 2);
    var S = STR[lang] || STR.en;
    window.evey.media.list().then(function (resp) {
      var items = (resp && resp.items) || [];
      var dups = findDuplicates(items);
      var holes = findSagaHoles(items);
      document.body.innerHTML = "";
      if (dups.length === 0 && holes.length === 0) {
        // A clean shelf earns silence, not a banner.
        window.evey.resize(0);
        return;
      }
      var card = h("div",
        "padding:10px 14px;border-radius:12px;background:var(--surface, rgba(127,127,127,.12));");
      var head = h("div", "display:flex;gap:10px;align-items:baseline;margin-bottom:4px;");
      head.appendChild(h("span", "font-size:14px;", "🗂️"));
      head.appendChild(h("span", "font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent, #7aa2f7);", S.title));
      var tally = [];
      if (dups.length) tally.push(S.dups(dups.length));
      if (holes.length) tally.push(S.holes(holes.length));
      head.appendChild(h("span", "font-size:12px;color:var(--text-muted, #9aa0aa);", tally.join(" · ")));
      card.appendChild(head);
      var i;
      for (i = 0; i < Math.min(2, dups.length); i++) {
        var d = dups[i];
        var extra = d.sameYear ? S.copies(d.entries.length) : STR[lang] ? S.remake : S.remake;
        card.appendChild(line("♻️", d.title + " - " + extra, d.entries[0].id));
      }
      for (i = 0; i < Math.min(2, holes.length); i++) {
        var g = holes[i];
        card.appendChild(line("🧩", g.sample.title + " - " + S.missing(g.missing), g.sample.id));
      }
      document.body.appendChild(card);
      window.evey.resize(document.body.scrollHeight);
    }).catch(function () {
      window.evey.resize(0);
    });
  });
})();
