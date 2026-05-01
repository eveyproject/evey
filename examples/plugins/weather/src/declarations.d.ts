// Asset type declarations for esbuild loaders
declare module "*.png" {
  const dataUrl: string;
  export default dataUrl;
}
