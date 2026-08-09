import { ReactElement } from "react";
import { Box } from "./box";
import Content from "./content";
import { Resource, resources as legacyResources } from "../data";
import { SponsorAd } from "../lib/catalog";

export default function Layout({ children, resources = legacyResources, ads = [] }: { children: ReactElement; resources?: Resource[]; ads?: SponsorAd[] }): JSX.Element {
    return (
        <Box
            css={{
                maxW: "100%",
            }}
        >
            {children}
            <Content resources={resources} ads={ads} />
        </Box>
    );
}
