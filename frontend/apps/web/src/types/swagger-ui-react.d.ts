// swagger-ui-react ships no type declarations. Declare the minimal surface we
// use (the package is consumed only by the API docs page) rather than pulling
// @types/swagger-ui-react, which can drag in a divergent @types/react.
declare module "swagger-ui-react" {
  import type { ComponentType } from "react";

  interface SwaggerUIProps {
    spec?: object;
    url?: string;
    docExpansion?: "list" | "full" | "none";
    requestInterceptor?: (req: unknown) => unknown;
    tryItOutEnabled?: boolean;
  }

  const SwaggerUI: ComponentType<SwaggerUIProps>;
  export default SwaggerUI;
}
