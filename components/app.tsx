import {Navbar, Button, Link, Text} from "@nextui-org/react";
import {NavItem, Resource, resources as legacyResources, site} from "../data";
import Layout from "./layout";
import {Image} from '@nextui-org/react';
import HeartIcon from "./heartIcon";
import { SponsorAd } from "../lib/catalog";

export default function App({resources = legacyResources, ads = []}: {resources?: Resource[]; ads?: SponsorAd[]}) {
    const color = 'error';
    const variant = 'default';

    return (
        <Layout resources={resources} ads={ads}>
            <Navbar variant="sticky">
                <Link href="/" color="inherit">
                    <Navbar.Brand>
                        {
                            (site.logo) &&
                            <Image src={site.logo} alt={site.seo.title} width={32} height={32}/>
                        }
                        <Text b color="inherit" hideIn="xs">
                            {site.seo.title}
                        </Text>
                    </Navbar.Brand>
                </Link>
                <Navbar.Content activeColor={color} hideIn="xs" variant={variant}>
                    {
                        site.navItems.map((item: NavItem, index: number) => {
                            const resource = resources.find((candidate) => candidate.title === item.name);
                            const href = resource ? `#${resource.id ?? resource.title}` : item.url;
                            return (
                                <Navbar.Link href={href} key={index}>{item.name}</Navbar.Link>
                            )
                        })
                    }
                </Navbar.Content>
                <Navbar.Content>
                    <Navbar.Item>
                        <Link href="https://github.com/sponsors/forecho" target="_blank" rel="noopener noreferrer">
                            <Button auto color={color} href="#" icon={<HeartIcon/>}>
                                Sponsor
                            </Button>
                        </Link>
                    </Navbar.Item>
                </Navbar.Content>
            </Navbar>
        </Layout>
    )
}
