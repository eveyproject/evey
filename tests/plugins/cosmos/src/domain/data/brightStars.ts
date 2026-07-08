// Curated catalog of well-known naked-eye stars (J2000 coordinates), covering
// the ~18 most recognizable constellations (see constellations.ts for the
// line figures). Coordinates are the commonly published values for these
// named stars (the kind found in the Yale Bright Star Catalogue / IAU star
// name lists), rounded to a few decimal places - plenty for a sky widget
// that only needs degree-level placement.
//
// This is a deliberately smaller, high-confidence set rather than the full
// ~300-star Yale Bright Star Catalogue: this plugin has no network access to
// fetch and cross-check the raw catalogue file, and fabricating precise
// numeric coordinates for stars beyond what is reliably known would be
// presenting invented data as real astronomy.
//
// id: stable slug referenced by constellations.ts line segments.
// raH: right ascension in decimal hours. decDeg: declination in decimal
// degrees. mag: apparent visual magnitude (lower = brighter). bv: B-V color
// index (negative = blue-white, ~0 = white, positive = orange/red).

export interface BrightStar {
  id: string;
  name: string;
  con: string; // IAU 3-letter constellation code
  raH: number;
  decDeg: number;
  mag: number;
  bv: number;
}

export const BRIGHT_STARS: BrightStar[] = [
  { id: "sirius", name: "Sirius", con: "CMa", raH: 6.7525, decDeg: -16.7161, mag: -1.46, bv: 0.0 },
  { id: "canopus", name: "Canopus", con: "Car", raH: 6.3992, decDeg: -52.6957, mag: -0.74, bv: 0.15 },
  { id: "rigil_kent", name: "Rigil Kentaurus", con: "Cen", raH: 14.66, decDeg: -60.8339, mag: -0.27, bv: 0.71 },
  { id: "arcturus", name: "Arcturus", con: "Boo", raH: 14.261, decDeg: 19.1825, mag: -0.05, bv: 1.23 },
  { id: "vega", name: "Vega", con: "Lyr", raH: 18.6157, decDeg: 38.7837, mag: 0.03, bv: 0.0 },
  { id: "capella", name: "Capella", con: "Aur", raH: 5.2782, decDeg: 45.998, mag: 0.08, bv: 0.8 },
  { id: "rigel", name: "Rigel", con: "Ori", raH: 5.2423, decDeg: -8.2016, mag: 0.13, bv: -0.03 },
  { id: "procyon", name: "Procyon", con: "CMi", raH: 7.655, decDeg: 5.225, mag: 0.34, bv: 0.42 },
  { id: "achernar", name: "Achernar", con: "Eri", raH: 1.6286, decDeg: -57.2367, mag: 0.46, bv: -0.16 },
  { id: "betelgeuse", name: "Betelgeuse", con: "Ori", raH: 5.9195, decDeg: 7.4071, mag: 0.5, bv: 1.85 },
  { id: "hadar", name: "Hadar", con: "Cen", raH: 14.0637, decDeg: -60.373, mag: 0.61, bv: -0.23 },
  { id: "altair", name: "Altair", con: "Aql", raH: 19.8464, decDeg: 8.8683, mag: 0.76, bv: 0.22 },
  { id: "acrux", name: "Acrux", con: "Cru", raH: 12.4433, decDeg: -63.0991, mag: 0.77, bv: -0.24 },
  { id: "aldebaran", name: "Aldebaran", con: "Tau", raH: 4.5987, decDeg: 16.5093, mag: 0.85, bv: 1.54 },
  { id: "antares", name: "Antares", con: "Sco", raH: 16.4901, decDeg: -26.432, mag: 0.96, bv: 1.83 },
  { id: "spica", name: "Spica", con: "Vir", raH: 13.4199, decDeg: -11.1613, mag: 0.97, bv: -0.24 },
  { id: "pollux", name: "Pollux", con: "Gem", raH: 7.7553, decDeg: 28.0262, mag: 1.14, bv: 1.0 },
  { id: "fomalhaut", name: "Fomalhaut", con: "PsA", raH: 22.9608, decDeg: -29.6222, mag: 1.16, bv: 0.09 },
  { id: "deneb", name: "Deneb", con: "Cyg", raH: 20.6905, decDeg: 45.2803, mag: 1.25, bv: 0.09 },
  { id: "mimosa", name: "Mimosa", con: "Cru", raH: 12.7953, decDeg: -59.6888, mag: 1.25, bv: -0.23 },
  { id: "regulus", name: "Regulus", con: "Leo", raH: 10.1395, decDeg: 11.9672, mag: 1.36, bv: -0.11 },
  { id: "adhara", name: "Adhara", con: "CMa", raH: 6.9771, decDeg: -28.9721, mag: 1.5, bv: -0.21 },
  { id: "castor", name: "Castor", con: "Gem", raH: 7.5766, decDeg: 31.8883, mag: 1.58, bv: 0.03 },
  { id: "gacrux", name: "Gacrux", con: "Cru", raH: 12.5194, decDeg: -57.1133, mag: 1.63, bv: 1.6 },
  { id: "shaula", name: "Shaula", con: "Sco", raH: 17.5601, decDeg: -37.1038, mag: 1.63, bv: -0.22 },
  { id: "bellatrix", name: "Bellatrix", con: "Ori", raH: 5.4188, decDeg: 6.3497, mag: 1.64, bv: -0.22 },
  { id: "elnath", name: "Elnath", con: "Tau", raH: 5.4382, decDeg: 28.6075, mag: 1.65, bv: -0.13 },
  { id: "miaplacidus", name: "Miaplacidus", con: "Car", raH: 9.2199, decDeg: -69.7172, mag: 1.67, bv: 0.0 },
  { id: "alnilam", name: "Alnilam", con: "Ori", raH: 5.6036, decDeg: -1.2019, mag: 1.69, bv: -0.18 },
  { id: "alnitak", name: "Alnitak", con: "Ori", raH: 5.6793, decDeg: -1.9426, mag: 1.77, bv: -0.2 },
  { id: "mintaka", name: "Mintaka", con: "Ori", raH: 5.5334, decDeg: -0.2991, mag: 2.23, bv: -0.19 },
  { id: "meissa", name: "Meissa", con: "Ori", raH: 5.5851, decDeg: 9.9342, mag: 3.54, bv: -0.18 },
  { id: "saiph", name: "Saiph", con: "Ori", raH: 5.7959, decDeg: -9.6696, mag: 2.07, bv: -0.17 },
  { id: "alioth", name: "Alioth", con: "UMa", raH: 12.9005, decDeg: 55.9598, mag: 1.76, bv: 0.0 },
  { id: "dubhe", name: "Dubhe", con: "UMa", raH: 11.0621, decDeg: 61.7511, mag: 1.79, bv: 1.07 },
  { id: "merak", name: "Merak", con: "UMa", raH: 11.0307, decDeg: 56.3824, mag: 2.34, bv: 0.03 },
  { id: "phecda", name: "Phecda", con: "UMa", raH: 11.8971, decDeg: 53.6948, mag: 2.44, bv: 0.0 },
  { id: "megrez", name: "Megrez", con: "UMa", raH: 12.257, decDeg: 57.0326, mag: 3.32, bv: 0.08 },
  { id: "mizar", name: "Mizar", con: "UMa", raH: 13.3987, decDeg: 54.9254, mag: 2.23, bv: 0.02 },
  { id: "alkaid", name: "Alkaid", con: "UMa", raH: 13.7923, decDeg: 49.3133, mag: 1.85, bv: -0.19 },
  { id: "mirfak", name: "Mirfak", con: "Per", raH: 3.4054, decDeg: 49.8612, mag: 1.79, bv: 0.48 },
  { id: "algol", name: "Algol", con: "Per", raH: 3.1361, decDeg: 40.9556, mag: 2.12, bv: -0.05 },
  { id: "atik", name: "Atik", con: "Per", raH: 3.9021, decDeg: 31.8836, mag: 2.85, bv: -0.19 },
  { id: "wezen", name: "Wezen", con: "CMa", raH: 7.1398, decDeg: -26.3932, mag: 1.83, bv: 0.68 },
  { id: "mirzam", name: "Mirzam", con: "CMa", raH: 6.3783, decDeg: -17.9559, mag: 1.98, bv: -0.13 },
  { id: "aludra", name: "Aludra", con: "CMa", raH: 7.4013, decDeg: -29.3031, mag: 2.45, bv: -0.09 },
  { id: "furud", name: "Furud", con: "CMa", raH: 6.3399, decDeg: -30.0631, mag: 3.02, bv: -0.19 },
  { id: "sargas", name: "Sargas", con: "Sco", raH: 17.6222, decDeg: -42.9978, mag: 1.87, bv: 0.4 },
  { id: "dschubba", name: "Dschubba", con: "Sco", raH: 16.0056, decDeg: -22.6217, mag: 2.29, bv: -0.12 },
  { id: "larawag", name: "Larawag", con: "Sco", raH: 16.8302, decDeg: -34.2932, mag: 2.29, bv: 1.15 },
  { id: "graffias", name: "Graffias", con: "Sco", raH: 16.0906, decDeg: -19.8054, mag: 2.56, bv: -0.07 },
  { id: "girtab", name: "Girtab", con: "Sco", raH: 17.7075, decDeg: -39.03, mag: 2.39, bv: -0.06 },
  { id: "lesath", name: "Lesath", con: "Sco", raH: 17.5128, decDeg: -37.2958, mag: 2.7, bv: -0.19 },
  { id: "kaus_australis", name: "Kaus Australis", con: "Sgr", raH: 18.4028, decDeg: -34.3846, mag: 1.85, bv: 0.03 },
  { id: "nunki", name: "Nunki", con: "Sgr", raH: 18.9211, decDeg: -26.2967, mag: 2.05, bv: -0.18 },
  { id: "kaus_media", name: "Kaus Media", con: "Sgr", raH: 18.3502, decDeg: -29.8281, mag: 2.7, bv: 1.39 },
  { id: "ascella", name: "Ascella", con: "Sgr", raH: 19.0433, decDeg: -29.8801, mag: 2.6, bv: 0.02 },
  { id: "kaus_borealis", name: "Kaus Borealis", con: "Sgr", raH: 18.4694, decDeg: -25.4217, mag: 2.82, bv: 1.04 },
  { id: "alnasl", name: "Alnasl", con: "Sgr", raH: 18.0973, decDeg: -30.4241, mag: 2.98, bv: 1.04 },
  { id: "avior", name: "Avior", con: "Car", raH: 8.3752, decDeg: -59.5095, mag: 1.86, bv: 1.28 },
  { id: "menkalinan", name: "Menkalinan", con: "Aur", raH: 5.9922, decDeg: 44.9474, mag: 1.9, bv: 0.03 },
  { id: "atria", name: "Atria", con: "TrA", raH: 16.8111, decDeg: -69.0277, mag: 1.91, bv: 1.44 },
  { id: "alhena", name: "Alhena", con: "Gem", raH: 6.6284, decDeg: 16.3993, mag: 1.93, bv: 0.0 },
  { id: "wasat", name: "Wasat", con: "Gem", raH: 7.3378, decDeg: 21.9825, mag: 3.53, bv: 0.33 },
  { id: "mebsuta", name: "Mebsuta", con: "Gem", raH: 6.7333, decDeg: 25.1311, mag: 3.06, bv: 1.44 },
  { id: "tejat", name: "Tejat", con: "Gem", raH: 6.383, decDeg: 22.5137, mag: 2.88, bv: 1.64 },
  { id: "peacock", name: "Peacock", con: "Pav", raH: 20.4275, decDeg: -56.735, mag: 1.94, bv: -0.12 },
  { id: "alsephina", name: "Alsephina", con: "Vel", raH: 8.7455, decDeg: -54.7086, mag: 1.96, bv: -0.03 },
  { id: "regor", name: "Regor", con: "Vel", raH: 8.1594, decDeg: -47.3365, mag: 1.75, bv: -0.19 },
  { id: "alphard", name: "Alphard", con: "Hya", raH: 9.46, decDeg: -8.6586, mag: 1.98, bv: 1.44 },
  { id: "hamal", name: "Hamal", con: "Ari", raH: 2.1196, decDeg: 23.4624, mag: 2.0, bv: 1.15 },
  { id: "polaris", name: "Polaris", con: "UMi", raH: 2.5303, decDeg: 89.2641, mag: 1.98, bv: 0.6 },
  { id: "kochab", name: "Kochab", con: "UMi", raH: 14.8451, decDeg: 74.1555, mag: 2.07, bv: 1.47 },
  { id: "pherkad", name: "Pherkad", con: "UMi", raH: 15.3455, decDeg: 71.834, mag: 3.05, bv: 0.05 },
  { id: "alnair", name: "Alnair", con: "Gru", raH: 22.1372, decDeg: -46.9611, mag: 1.74, bv: -0.07 },
  { id: "almach", name: "Almach", con: "And", raH: 2.065, decDeg: 42.3297, mag: 2.1, bv: 1.37 },
  { id: "alpheratz", name: "Alpheratz", con: "And", raH: 0.1398, decDeg: 29.0904, mag: 2.06, bv: -0.03 },
  { id: "mirach", name: "Mirach", con: "And", raH: 1.1622, decDeg: 35.6206, mag: 2.05, bv: 1.58 },
  { id: "denebola", name: "Denebola", con: "Leo", raH: 11.8177, decDeg: 14.572, mag: 2.14, bv: 0.09 },
  { id: "algieba", name: "Algieba", con: "Leo", raH: 10.3327, decDeg: 19.8415, mag: 2.28, bv: 1.14 },
  { id: "zosma", name: "Zosma", con: "Leo", raH: 11.2351, decDeg: 20.5237, mag: 2.56, bv: 0.13 },
  { id: "chertan", name: "Chertan", con: "Leo", raH: 11.2373, decDeg: 15.4296, mag: 3.34, bv: 0.03 },
  { id: "muhlifain", name: "Muhlifain", con: "Cen", raH: 12.6923, decDeg: -48.9598, mag: 2.2, bv: 0.0 },
  { id: "menkent", name: "Menkent", con: "Cen", raH: 14.1113, decDeg: -36.3699, mag: 2.06, bv: 0.98 },
  { id: "alphecca", name: "Alphecca", con: "CrB", raH: 15.5782, decDeg: 26.7147, mag: 2.23, bv: 0.03 },
  { id: "sadr", name: "Sadr", con: "Cyg", raH: 20.3705, decDeg: 40.2567, mag: 2.23, bv: 0.67 },
  { id: "aljanah", name: "Aljanah", con: "Cyg", raH: 20.7702, decDeg: 33.9702, mag: 2.46, bv: 1.03 },
  { id: "fawaris", name: "Fawaris", con: "Cyg", raH: 19.7497, decDeg: 45.131, mag: 2.87, bv: 0.05 },
  { id: "albireo", name: "Albireo", con: "Cyg", raH: 19.5121, decDeg: 27.9597, mag: 3.18, bv: 1.14 },
  { id: "eltanin", name: "Eltanin", con: "Dra", raH: 17.9434, decDeg: 51.4889, mag: 2.23, bv: 1.53 },
  { id: "caph", name: "Caph", con: "Cas", raH: 0.1529, decDeg: 59.1498, mag: 2.27, bv: 0.38 },
  { id: "schedar", name: "Schedar", con: "Cas", raH: 0.6751, decDeg: 56.5373, mag: 2.24, bv: 1.17 },
  { id: "tsih", name: "Tsih", con: "Cas", raH: 0.9451, decDeg: 60.7167, mag: 2.47, bv: -0.15 },
  { id: "ruchbah", name: "Ruchbah", con: "Cas", raH: 1.43, decDeg: 60.2353, mag: 2.68, bv: 0.15 },
  { id: "segin", name: "Segin", con: "Cas", raH: 1.9061, decDeg: 63.6701, mag: 3.35, bv: -0.11 },
  { id: "ankaa", name: "Ankaa", con: "Phe", raH: 0.4383, decDeg: -42.3061, mag: 2.4, bv: 1.08 },
  { id: "enif", name: "Enif", con: "Peg", raH: 21.7364, decDeg: 9.875, mag: 2.38, bv: 1.52 },
  { id: "scheat", name: "Scheat", con: "Peg", raH: 23.0629, decDeg: 28.0828, mag: 2.42, bv: 1.67 },
  { id: "markab", name: "Markab", con: "Peg", raH: 23.0794, decDeg: 15.2053, mag: 2.49, bv: -0.04 },
  { id: "sabik", name: "Sabik", con: "Oph", raH: 17.1728, decDeg: -15.7249, mag: 2.43, bv: -0.02 },
  { id: "rasalhague", name: "Rasalhague", con: "Oph", raH: 17.5824, decDeg: 12.5601, mag: 2.08, bv: 0.15 },
  { id: "alderamin", name: "Alderamin", con: "Cep", raH: 21.3096, decDeg: 62.5856, mag: 2.45, bv: 0.22 },
  { id: "errai", name: "Errai", con: "Cep", raH: 23.6564, decDeg: 77.632, mag: 3.21, bv: 1.03 },
  { id: "zubenelgenubi", name: "Zubenelgenubi", con: "Lib", raH: 14.8479, decDeg: -16.0418, mag: 2.75, bv: 0.15 },
  { id: "zubeneschamali", name: "Zubeneschamali", con: "Lib", raH: 15.2833, decDeg: -9.383, mag: 2.61, bv: -0.03 },
  { id: "unukalhai", name: "Unukalhai", con: "Ser", raH: 15.7382, decDeg: 6.4256, mag: 2.63, bv: 1.17 },
  { id: "menkar", name: "Menkar", con: "Cet", raH: 3.0378, decDeg: 4.09, mag: 2.54, bv: 1.64 },
  { id: "diphda", name: "Diphda", con: "Cet", raH: 0.7265, decDeg: -17.9866, mag: 2.04, bv: 1.02 },
  { id: "izar", name: "Izar", con: "Boo", raH: 14.75, decDeg: 27.0742, mag: 2.35, bv: 0.96 },
  { id: "seginus", name: "Seginus", con: "Boo", raH: 14.535, decDeg: 38.3082, mag: 3.03, bv: 0.05 },
  { id: "nekkar", name: "Nekkar", con: "Boo", raH: 15.0322, decDeg: 40.3906, mag: 3.5, bv: 0.95 },
  { id: "muphrid", name: "Muphrid", con: "Boo", raH: 13.9144, decDeg: 18.3976, mag: 2.68, bv: 0.58 },
  { id: "sheliak", name: "Sheliak", con: "Lyr", raH: 18.8347, decDeg: 33.3627, mag: 3.45, bv: 0.0 },
  { id: "sulafat", name: "Sulafat", con: "Lyr", raH: 18.9825, decDeg: 32.6896, mag: 3.24, bv: -0.05 },
  { id: "tarazed", name: "Tarazed", con: "Aql", raH: 19.7709, decDeg: 10.6133, mag: 2.72, bv: 1.53 },
  { id: "alshain", name: "Alshain", con: "Aql", raH: 19.9226, decDeg: 6.4067, mag: 3.71, bv: 0.86 },
  { id: "imai", name: "Imai", con: "Cru", raH: 12.2528, decDeg: -58.7489, mag: 2.79, bv: -0.13 },
  { id: "alcyone", name: "Alcyone", con: "Tau", raH: 3.7912, decDeg: 24.1051, mag: 2.87, bv: -0.09 },
];
