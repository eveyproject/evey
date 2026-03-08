import { jsx as _jsx } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { MediaDetail } from "@/features/library/MediaDetail";
export const Route = createFileRoute("/_auth/media/$mediaId")({
    component: MediaPage,
});
function MediaPage() {
    const { mediaId } = Route.useParams();
    const { data: item, isLoading, isError } = useQuery({
        queryKey: [`/api/v1/media/${mediaId}`],
        queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaId, signal),
        enabled: !!mediaId,
    });
    if (isLoading) {
        return (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) }));
    }
    if (isError || !item) {
        return (_jsx("div", { className: "text-center py-16", style: { color: "var(--text-muted)" }, children: "Media item not found." }));
    }
    return _jsx(MediaDetail, { item: item });
}
