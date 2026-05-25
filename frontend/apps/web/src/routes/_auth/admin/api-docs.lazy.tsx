import { createLazyFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetchRaw } from "@evey/api";
import SwaggerUI from "swagger-ui-react";
import "swagger-ui-react/swagger-ui.css";

export const Route = createLazyFileRoute("/_auth/admin/api-docs")({
  component: ApiDocsPage,
});

function ApiDocsPage() {
  const { t } = useTranslation();
  const [spec, setSpec] = useState<object | null>(null);
  const [error, setError] = useState(false);

  // The spec endpoint is behind auth, so a raw SwaggerUI url= fetch would 401.
  // Load it through eveyFetchRaw (joins the bearer token) and hand SwaggerUI
  // the parsed object via the `spec` prop instead.
  useEffect(() => {
    let active = true;
    eveyFetchRaw({ url: "/api/v1/openapi.json", method: "GET" })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((s) => active && setSpec(s as object))
      .catch(() => active && setError(true));
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="px-6 py-8 max-w-5xl flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("auth.apiDocs.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mt-1">{t("auth.apiDocs.subtitle")}</p>
      </div>
      {error && <p className="text-sm text-[var(--danger)]">{t("auth.apiDocs.loadError")}</p>}
      {spec && (
        <div className="bg-white rounded-[var(--radius-md)] overflow-hidden">
          <SwaggerUI spec={spec} docExpansion="list" />
        </div>
      )}
    </div>
  );
}
