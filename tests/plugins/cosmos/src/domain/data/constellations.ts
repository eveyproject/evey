// Stick-figure line segments for the ~18 most recognizable constellations
// (exceeds the requested "15 major constellations"). Each segment is a pair
// of star ids from brightStars.ts; the renderer looks up each star's current
// projected screen position and draws a line between them.

export interface Constellation {
  id: string;
  labelKey: string; // i18n key, see i18n/labels.ts
  segments: Array<[string, string]>;
}

export const CONSTELLATIONS: Constellation[] = [
  {
    id: "UMa",
    labelKey: "con_ursa_major",
    segments: [
      ["alkaid", "mizar"],
      ["mizar", "alioth"],
      ["alioth", "megrez"],
      ["megrez", "phecda"],
      ["phecda", "merak"],
      ["merak", "dubhe"],
      ["dubhe", "megrez"],
    ],
  },
  {
    id: "UMi",
    labelKey: "con_ursa_minor",
    segments: [
      ["polaris", "pherkad"],
      ["pherkad", "kochab"],
    ],
  },
  {
    id: "Ori",
    labelKey: "con_orion",
    segments: [
      ["betelgeuse", "bellatrix"],
      ["bellatrix", "mintaka"],
      ["betelgeuse", "alnitak"],
      ["mintaka", "alnilam"],
      ["alnilam", "alnitak"],
      ["mintaka", "rigel"],
      ["alnitak", "saiph"],
      ["bellatrix", "meissa"],
      ["betelgeuse", "meissa"],
    ],
  },
  {
    id: "Cas",
    labelKey: "con_cassiopeia",
    segments: [
      ["segin", "ruchbah"],
      ["ruchbah", "tsih"],
      ["tsih", "schedar"],
      ["schedar", "caph"],
    ],
  },
  {
    id: "Cyg",
    labelKey: "con_cygnus",
    segments: [
      ["deneb", "sadr"],
      ["sadr", "aljanah"],
      ["sadr", "fawaris"],
      ["sadr", "albireo"],
    ],
  },
  {
    id: "Leo",
    labelKey: "con_leo",
    segments: [
      ["denebola", "chertan"],
      ["chertan", "zosma"],
      ["chertan", "algieba"],
      ["algieba", "regulus"],
    ],
  },
  {
    id: "Sco",
    labelKey: "con_scorpius",
    segments: [
      ["graffias", "dschubba"],
      ["dschubba", "antares"],
      ["antares", "larawag"],
      ["larawag", "sargas"],
      ["sargas", "girtab"],
      ["girtab", "shaula"],
      ["shaula", "lesath"],
    ],
  },
  {
    id: "Sgr",
    labelKey: "con_sagittarius",
    segments: [
      ["kaus_borealis", "kaus_media"],
      ["kaus_media", "kaus_australis"],
      ["kaus_australis", "alnasl"],
      ["kaus_borealis", "nunki"],
      ["nunki", "ascella"],
    ],
  },
  {
    id: "Tau",
    labelKey: "con_taurus",
    segments: [
      ["aldebaran", "elnath"],
      ["aldebaran", "alcyone"],
    ],
  },
  {
    id: "Gem",
    labelKey: "con_gemini",
    segments: [
      ["castor", "pollux"],
      ["castor", "wasat"],
      ["wasat", "alhena"],
      ["pollux", "mebsuta"],
      ["mebsuta", "tejat"],
    ],
  },
  {
    id: "CMa",
    labelKey: "con_canis_major",
    segments: [
      ["sirius", "mirzam"],
      ["sirius", "adhara"],
      ["adhara", "wezen"],
      ["wezen", "aludra"],
      ["sirius", "furud"],
    ],
  },
  {
    id: "Cru",
    labelKey: "con_crux",
    segments: [
      ["acrux", "gacrux"],
      ["mimosa", "imai"],
    ],
  },
  {
    id: "Cen",
    labelKey: "con_centaurus",
    segments: [
      ["rigil_kent", "hadar"],
      ["hadar", "menkent"],
      ["hadar", "muhlifain"],
    ],
  },
  {
    id: "Boo",
    labelKey: "con_bootes",
    segments: [
      ["arcturus", "muphrid"],
      ["arcturus", "seginus"],
      ["seginus", "nekkar"],
      ["arcturus", "izar"],
    ],
  },
  {
    id: "Lyr",
    labelKey: "con_lyra",
    segments: [
      ["vega", "sheliak"],
      ["sheliak", "sulafat"],
    ],
  },
  {
    id: "Aql",
    labelKey: "con_aquila",
    segments: [
      ["tarazed", "altair"],
      ["altair", "alshain"],
    ],
  },
  {
    id: "And",
    labelKey: "con_andromeda",
    segments: [
      ["alpheratz", "mirach"],
      ["mirach", "almach"],
    ],
  },
  {
    id: "Per",
    labelKey: "con_perseus",
    segments: [
      ["mirfak", "algol"],
      ["mirfak", "atik"],
    ],
  },
];
