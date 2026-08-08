import { createLazyFileRoute } from "@tanstack/react-router";
import { IntroTestLab } from "@/features/admin/IntroTestLab";

export const Route = createLazyFileRoute("/_auth/admin/intro-test")({
  component: IntroTestLab,
});
