// Shared implementation lives in @evey/client (packages/client/src/api/collections.ts),
// the same layer the phone and the televisions use. This module keeps the path
// the web's screens and tests import; the web pages a collection with a
// numbered pager, so its useCollection is the paged read.
export {
  useCollections,
  useAllCollections,
  useCollectionPage as useCollection,
  useCreateCollection,
  useUpdateCollection,
  useDeleteCollection,
  addCollectionItem,
  useRemoveCollectionItem,
  useReorderCollectionItems,
} from "@evey/client";
export type { Collection, CollectionItem, CollectionRule } from "@evey/client";
