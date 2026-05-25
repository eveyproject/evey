import { createLazyFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetchRaw } from "@evey/api";
import SwaggerUI from "swagger-ui-react";
import "swagger-ui-react/swagger-ui.css";

export const Route = createLazyFileRoute("/_auth/admin/api-docs")({
  component: ApiDocsPage,
});

// Within each resource group, order operations by the classic REST verb order.
const METHOD_ORDER: Record<string, number> = { get: 0, post: 1, put: 2, patch: 3, delete: 4 };

function ApiDocsPage() {
  const { t } = useTranslation();
  const [spec, setSpec] = useState<object | null>(null);
  const [errMsg, setErrMsg] = useState("");

  // The spec endpoint is behind auth, so a raw SwaggerUI url= fetch would 401.
  // Load it through eveyFetchRaw (joins the bearer token) and hand SwaggerUI
  // the parsed object via the `spec` prop instead. eveyFetchRaw throws on a
  // non-2xx response (with httpStatus), so surface that for diagnosis.
  useEffect(() => {
    let active = true;
    eveyFetchRaw({ url: "/api/v1/openapi.json", method: "GET" })
      .then((res) => res.json())
      .then((s) => active && setSpec(s as object))
      .catch((e: unknown) => {
        if (active) setErrMsg(e instanceof Error ? e.message : String(e));
      });
    return () => {
      active = false;
    };
  }, []);
  const error = errMsg !== "";

  return (
    <div className="px-6 py-8 max-w-5xl flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.apiDocs.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mt-1">{t("auth.apiDocs.subtitle")}</p>
      </div>
      {error && (
        <p className="text-sm text-[var(--danger)]">
          {t("auth.apiDocs.loadError")} <span className="font-mono text-xs opacity-80">({errMsg})</span>
        </p>
      )}
      {spec && (
        <div className="bg-white rounded-[var(--radius-md)] overflow-hidden">
          <SwaggerUI
            spec={spec}
            docExpansion="list"
            operationsSorter={(a, b) => {
              const d = (METHOD_ORDER[a.get("method")] ?? 9) - (METHOD_ORDER[b.get("method")] ?? 9);
              return d !== 0 ? d : a.get("path").localeCompare(b.get("path"));
            }}
          />
        </div>
      )}
    </div>
  );
}
