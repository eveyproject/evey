// Shared millisecond-unit time constants, to avoid re-deriving 3600000/86400000
// magic numbers across stats/history formatters.
export const MS_PER_MINUTE = 60_000;
export const MS_PER_HOUR = 60 * MS_PER_MINUTE;
export const MS_PER_DAY = 24 * MS_PER_HOUR;
