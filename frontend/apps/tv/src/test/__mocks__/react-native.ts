export const Platform = { OS: "android", select: (obj: Record<string, unknown>) => obj.android };

export const Dimensions = {
  get: () => ({ width: 1920, height: 1080, scale: 1 }),
};
