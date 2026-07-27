// Shared between the region settings screen and the profile edit dialog so
// the two pickers never drift (one previously used a truncated fallback list
// missing dozens of countries while the other had the full ISO-3166 set).

// Fallback list used when Hermes/JSC doesn't expose Intl.supportedValuesOf.
const ISO_REGIONS = [
  "AD","AE","AF","AG","AI","AL","AM","AO","AR","AS","AT","AU","AW","AX","AZ",
  "BA","BB","BD","BE","BF","BG","BH","BI","BJ","BL","BM","BN","BO","BQ","BR",
  "BS","BT","BV","BW","BY","BZ","CA","CC","CD","CF","CG","CH","CI","CK","CL",
  "CM","CN","CO","CR","CU","CV","CW","CX","CY","CZ","DE","DJ","DK","DM","DO",
  "DZ","EC","EE","EG","EH","ER","ES","ET","FI","FJ","FK","FM","FO","FR","GA",
  "GB","GD","GE","GF","GG","GH","GI","GL","GM","GN","GP","GQ","GR","GS","GT",
  "GU","GW","GY","HK","HM","HN","HR","HT","HU","ID","IE","IL","IM","IN","IO",
  "IQ","IR","IS","IT","JE","JM","JO","JP","KE","KG","KH","KI","KM","KN","KP",
  "KR","KW","KY","KZ","LA","LB","LC","LI","LK","LR","LS","LT","LU","LV","LY",
  "MA","MC","MD","ME","MF","MG","MH","MK","ML","MM","MN","MO","MP","MQ","MR",
  "MS","MT","MU","MV","MW","MX","MY","MZ","NA","NC","NE","NF","NG","NI","NL",
  "NO","NP","NR","NU","NZ","OM","PA","PE","PF","PG","PH","PK","PL","PM","PN",
  "PR","PS","PT","PW","PY","QA","RE","RO","RS","RU","RW","SA","SB","SC","SD",
  "SE","SG","SH","SI","SJ","SK","SL","SM","SN","SO","SR","SS","ST","SV","SX",
  "SY","SZ","TC","TD","TF","TG","TH","TJ","TK","TL","TM","TN","TO","TR","TT",
  "TV","TW","TZ","UA","UG","UM","US","UY","UZ","VA","VC","VE","VG","VI","VN",
  "VU","WF","WS","XK","YE","YT","ZA","ZM","ZW",
];

export interface RegionOption {
  code: string;
  label: string;
}

export function regionOptions(locale: string): RegionOption[] {
  try {
    const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
    const dynamic: string[] = typeof sv === "function" ? sv("region") : [];
    const codes = dynamic.length > 0 ? dynamic : ISO_REGIONS;
    const names = new Intl.DisplayNames([locale], { type: "region" });
    return codes
      .filter((c) => /^[A-Z]{2}$/.test(c))
      .map((c) => ({ code: c, label: names.of(c) ?? c }))
      .sort((a, b) => a.label.localeCompare(b.label, locale));
  } catch {
    return ISO_REGIONS.map((c) => ({ code: c, label: c }));
  }
}
