import Banner from "./banner";
import {Text, Grid, Link} from "@nextui-org/react";
import {Resource, resources as legacyResources} from "../data";
import Item from "./item";
import {Box} from "./box";
import Footer from "./footer";
import SponsorAds from "./sponsorAds";
import { SponsorAd } from "../lib/catalog";
import { Fragment } from "react";

export default function Content({resources = legacyResources, ads = []}: {resources?: Resource[]; ads?: SponsorAd[]}) {
    return (
        <Box css={{px: "$12", mt: "$8", "@xsMax": {px: "$10"}}}>
            <Banner/>
            <Grid.Container gap={2}>
                <SponsorAds ads={ads}/>
                {
                    resources.map((resource, i) => {
                        return (
                            <Fragment key={resource.id ?? resource.title}>
								<Grid xs={12} id={resource.id ?? resource.title} className="title">
									<Link href={`#${resource.id ?? resource.title}`}>
                                        <Text h3>
                                            {resource.title}
                                        </Text>
                                    </Link>
                                </Grid>
                                {
                                    resource.site.map((site) => {
                                        return (
                                            <Grid xs={12} sm={6} md={4} lg={3} key={site.id ?? site.url}>
                                                <Item {...site}/>
                                            </Grid>
                                        )
                                    })
                                }
                            </Fragment>
                        )
                    })
                }
                <Footer/>
            </Grid.Container>

        </Box>
    );
}
