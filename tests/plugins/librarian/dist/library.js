// Librarian - library.sidebar module: this library's own findings, compact.
// Self-contained IIFE; the detection block is inlined VERBATIM from scan.js -
// the test suite fails on drift.
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
      title: "Bibliothécaire",
      dup: function (t, n) { return t + " est présent " + n + " fois"; },
      hole: function (t, nums) { return t + " : il manque " + (nums.length > 1 ? "les épisodes " : "l'épisode ") + nums.join(", "); },
    },
    en: {
      title: "Librarian",
      dup: function (t, n) { return t + " appears " + n + " times"; },
      hole: function (t, nums) { return t + ": missing " + (nums.length > 1 ? "episodes " : "episode ") + nums.join(", "); },
    },
  };

  function h(tag, style, text) {
    var el = document.createElement(tag);
    if (style) el.setAttribute("style", style);
    if (text) el.textContent = text;
    return el;
  }

  window.evey.onInit(function (init) {
    var lang = String(init.locale || "en").slice(0, 2);
    var S = STR[lang] || STR.en;
    var libraryId = init.context && init.context.libraryId ? String(init.context.libraryId) : "";
    window.evey.media.list(libraryId ? { libraryId: libraryId } : {}).then(function (resp) {
      var items = (resp && resp.items) || [];
      var dups = findDuplicates(items);
      var holes = findSagaHoles(items);
      document.body.innerHTML = "";
      if (dups.length === 0 && holes.length === 0) {
        window.evey.resize(0);
        return;
      }
      var strip = h("div",
        "display:flex;flex-direction:column;gap:2px;padding:8px 12px;border-radius:10px;margin-bottom:8px;" +
        "background:var(--surface, rgba(127,127,127,.12));font-size:12px;");
    var head = h("div", "display:flex;gap:8px;align-items:baseline;");
      head.appendChild(h("span", "", "🗂️"));
      head.appendChild(h("span", "font-size:10px;letter-spacing:.06em;text-transform:uppercase;color:var(--accent, #7aa2f7);", S.title));
      strip.appendChild(head);
      var shown = 0;
      var add = function (icon, text, id) {
        if (shown >= 4) return;
        shown++;
        var row = h("div", "display:flex;gap:6px;align-items:center;cursor:pointer;color:var(--text, #f1f5f9);");
        row.appendChild(h("span", "", icon));
        row.appendChild(h("span", "overflow:hidden;text-overflow:ellipsis;white-space:nowrap;", text));
        row.onclick = function () { window.evey.navigate("/media/" + id); };
        strip.appendChild(row);
      };
      var i;
      for (i = 0; i < dups.length; i++) add("♻️", S.dup(dups[i].title, dups[i].entries.length), dups[i].entries[0].id);
      for (i = 0; i < holes.length; i++) add("🧩", S.hole(holes[i].sample.title, holes[i].missing), holes[i].sample.id);
      document.body.appendChild(strip);
      window.evey.resize(document.body.scrollHeight);
    }).catch(function () {
      window.evey.resize(0);
    });
  });
})();
