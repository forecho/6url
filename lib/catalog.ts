import { resources as legacyResources, Resource } from "../data";

export interface SponsorAd {
  id: number;
  title: string;
  description: string;
  ctaLabel: string;
  altText: string;
  imageUrl: string;
  targetUrl: string;
  rel: string;
  label: string;
}

export interface HomeCatalog {
  resources: Resource[];
  ads: SponsorAd[];
  source: "database" | "json";
}

interface APICatalog {
  locale: string;
  categories: Array<{
    id: number;
    slug: string;
    title: string;
    description: string;
    sites: Array<{
      id: number;
      slug: string;
      name: string;
      tagline: string;
      logo_url?: string;
      outbound_url: string;
      outbound_rel: string;
    }>;
  }>;
  ads: Array<{
    id: number;
    title: string;
    description: string;
    cta_label: string;
    alt_text: string;
    image_url: string;
    target_url: string;
    rel: string;
    label: string;
  }>;
}

export async function loadHomeCatalog(locale = "zh-CN"): Promise<HomeCatalog> {
  if (process.env.SIXURL_CATALOG_SOURCE !== "database") {
    return legacyCatalog();
  }

  const apiURL = process.env.SIXURL_API_URL?.replace(/\/$/, "");
  if (!apiURL) {
    throw new Error("SIXURL_API_URL is required when SIXURL_CATALOG_SOURCE=database");
  }

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(`${apiURL}/api/v1/catalog?locale=${encodeURIComponent(locale)}`, {
      headers: { Accept: "application/json" },
      signal: controller.signal,
    });
    if (!response.ok) {
      throw new Error(`catalog API returned ${response.status}`);
    }
    const catalog = (await response.json()) as APICatalog;
    return {
      source: "database",
      resources: catalog.categories.map((category) => ({
        id: category.slug,
        title: category.title,
        description: category.description,
        icon: "",
        site: category.sites.map((site) => ({
          id: site.slug,
          title: site.name,
          description: site.tagline,
          url: site.outbound_url,
          logo: site.logo_url ?? "",
          rel: site.outbound_rel,
        })),
      })),
      ads: catalog.ads.map((ad) => ({
        id: ad.id,
        title: ad.title,
        description: ad.description,
        ctaLabel: ad.cta_label,
        altText: ad.alt_text,
        imageUrl: ad.image_url,
        targetUrl: ad.target_url,
        rel: ad.rel,
        label: ad.label,
      })),
    };
  } catch (error) {
    console.error("catalog API unavailable; serving the checked-in JSON snapshot", error);
    return legacyCatalog();
  } finally {
    clearTimeout(timeout);
  }
}

function legacyCatalog(): HomeCatalog {
  return { resources: legacyResources, ads: [], source: "json" };
}
