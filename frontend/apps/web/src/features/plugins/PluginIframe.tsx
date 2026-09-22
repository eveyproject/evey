// The iframe host lives in @evey/client (packages/client/src/web/PluginIframe.tsx):
// one host for the browser app and the browser television. This module keeps
// the path the slot and the tests import, and hands it this app's router.
import { useNavigate } from "@tanstack/react-router";
import { PluginIframe as SharedPluginIframe, isAllowedNavPath, suggestedSlotHeight } from "@evey/client/web";

export { isAllowedNavPath, suggestedSlotHeight };

type Props = Omit<Parameters<typeof SharedPluginIframe>[0], "onNavigate">;

export function PluginIframe(props: Props) {
  const navigate = useNavigate();
  return <SharedPluginIframe {...props} onNavigate={(path) => navigate({ to: path })} />;
}
