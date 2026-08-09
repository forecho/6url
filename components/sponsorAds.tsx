import Image from "next/image";
import { Card, Grid, Link, Text } from "@nextui-org/react";

import { SponsorAd } from "../lib/catalog";

export default function SponsorAds({ ads = [] }: { ads?: SponsorAd[] }) {
  if (ads.length === 0) {
    return null;
  }

  return (
    <>
      <Grid xs={12} className="title">
        <Text h3>Sponsor</Text>
      </Grid>
      {ads.map((ad) => (
        <Grid xs={12} sm={6} md={4} lg={3} key={ad.id}>
          <Card isPressable css={{ p: "$6", mw: "400px" }}>
            <Link href={ad.targetUrl} target="_blank" rel={ad.rel}>
              <Card.Header>
                <Image alt={ad.altText} src={ad.imageUrl} width="34" height="34" />
                <Grid.Container css={{ pl: "$6" }}>
                  <Grid xs={12}>
                    <Text small color="warning">{ad.label}</Text>
                  </Grid>
                  <Grid xs={12}>
                    <Text h4 css={{ lineHeight: "$xs" }}>{ad.title}</Text>
                  </Grid>
                  <Grid xs={12}>
                    <Text css={{ color: "$accents8" }}>{ad.description}</Text>
                  </Grid>
                </Grid.Container>
              </Card.Header>
            </Link>
          </Card>
        </Grid>
      ))}
    </>
  );
}
