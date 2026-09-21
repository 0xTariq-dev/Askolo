import { runtimeEnvironment } from './runtime-environment';

type PageMetadata = {
  title: string;
  description: string;
  canonicalPath?: string;
  robots?: string;
};

function setMetaContent(name: string, content: string): void {
  const selector = `meta[name="${name}"]`;
  let meta = document.querySelector<HTMLMetaElement>(selector);

  if (!meta) {
    meta = document.createElement('meta');
    meta.name = name;
    document.head.appendChild(meta);
  }

  meta.content = content;
}

function setPropertyContent(property: string, content: string): void {
  const selector = `meta[property="${property}"]`;
  let meta = document.querySelector<HTMLMetaElement>(selector);

  if (!meta) {
    meta = document.createElement('meta');
    meta.setAttribute('property', property);
    document.head.appendChild(meta);
  }

  meta.content = content;
}

function setCanonicalUrl(url: string): void {
  let canonical = document.querySelector<HTMLLinkElement>('link[rel="canonical"]');

  if (!canonical) {
    canonical = document.createElement('link');
    canonical.rel = 'canonical';
    document.head.appendChild(canonical);
  }

  canonical.href = url;
}

export function setPageMetadata({
  title,
  description,
  canonicalPath = '/',
  robots = runtimeEnvironment.isProduction ? 'index, follow' : 'noindex, nofollow',
}: PageMetadata): void {
  const canonicalUrl = new URL(canonicalPath, runtimeEnvironment.publicOrigin || window.location.origin).toString();

  document.title = title;
  setMetaContent('description', description);
  setMetaContent('robots', robots);
  setPropertyContent('og:title', title);
  setPropertyContent('og:description', description);
  setPropertyContent('og:url', canonicalUrl);
  setCanonicalUrl(canonicalUrl);
}