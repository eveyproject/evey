const STATIC_CACHE = "evey-static-v1";
const IMAGE_CACHE  = "evey-images-v1";
const IMAGE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

self.addEventListener("install", (e) => {
  e.waitUntil(self.skipWaiting());
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((k) => k !== STATIC_CACHE && k !== IMAGE_CACHE)
          .map((k) => caches.delete(k))
      )
    ).then(() => self.clients.claim())
  );
});

function isStaticAsset(url) {
  return /\.(js|css|woff2?|ttf|otf|eot)(\?.*)?$/.test(url.pathname);
}

function isImage(url) {
  return /\.(png|jpe?g|gif|svg|webp|ico)(\?.*)?$/.test(url.pathname);
}

function isApiRequest(url) {
  return url.pathname.startsWith("/api/");
}

async function staleWhileRevalidate(cacheName, request) {
  const cache = await caches.open(cacheName);
  const cached = await cache.match(request);
  const fetchPromise = fetch(request).then((res) => {
    if (res.ok) cache.put(request, res.clone());
    return res;
  });
  return cached ?? fetchPromise;
}

async function imageHandler(request) {
  const cache = await caches.open(IMAGE_CACHE);
  const cached = await cache.match(request);
  if (cached) {
    const date = cached.headers.get("sw-cached-at");
    if (date && Date.now() - Number(date) < IMAGE_TTL_MS) return cached;
  }
  const res = await fetch(request);
  if (res.ok) {
    const headers = new Headers(res.headers);
    headers.set("sw-cached-at", String(Date.now()));
    const stamped = new Response(await res.clone().arrayBuffer(), { status: res.status, headers });
    await cache.put(request, stamped);
  }
  return res;
}

self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== "GET" || isApiRequest(url)) return;
  if (isStaticAsset(url)) {
    e.respondWith(staleWhileRevalidate(STATIC_CACHE, e.request));
  } else if (isImage(url)) {
    e.respondWith(imageHandler(e.request));
  }
});

self.addEventListener("push", function (event) {
  const data = event.data?.json() ?? {};
  const title = data.title ?? "Evey";
  const options = {
    body: data.body ?? "",
    icon: "/favicon.ico",
    data: data.data ?? {},
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener("notificationclick", function (event) {
  event.notification.close();
  const url = event.notification.data?.url ?? "/";
  event.waitUntil(clients.openWindow(url));
});
