// The tsconfig keeps "types": [] on purpose - the front code must stay
// DOM-pure, with no Node globals leaking into src/. The backend-guard tests
// are the one place touching Node builtins and jpeg-js, so the minimal
// surface they use is declared here instead of pulling in @types/node.
declare module "fs" {
  export function readFileSync(path: string): Uint8Array;
}
declare module "path" {
  export function dirname(p: string): string;
  export function join(...parts: string[]): string;
}
declare module "url" {
  export function fileURLToPath(u: string): string;
}

// jpeg-js ships no type declarations; only the two entry points the tests and
// the backend guard rely on are typed here.
declare module "jpeg-js" {
  export interface RawImageData {
    width: number;
    height: number;
    data: Uint8Array;
  }
  export function decode(
    buf: ArrayLike<number>,
    opts?: { useTArray?: boolean; maxMemoryUsageInMB?: number },
  ): RawImageData;
  export function encode(
    img: { width: number; height: number; data: ArrayLike<number> },
    quality?: number,
  ): { width: number; height: number; data: Uint8Array };
}
