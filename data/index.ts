import siteJson from "./site.json";
import resourcesJson from "./resources.json";
import { DefaultSeoProps } from "next-seo";

export interface Site {
  logo?: string;
  keywords: string;
  favicon: string;
  seo: DefaultSeoProps;
  navItems: NavItem[];
  banner: {
    title: string;
    subtitle: string;
  }
}
export interface ResourceItem {
  id?: string;
  title: string;
  description?: string;
  url: string;
  logo: string;
  rel?: string;
}
export interface Resource {
  id?: string;
  title: string;
  description?: string;
  site: ResourceItem[];
  icon: string;
}

export interface NavItem {
  name: string;
  url: string;
}

export const site = siteJson as Site;
export const resources = resourcesJson as Resource[];
