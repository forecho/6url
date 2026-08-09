import Head from 'next/head'
import { GetStaticProps } from 'next';
import {site,} from "../data";
import App from '../components/app';
import { HomeCatalog, loadHomeCatalog } from '../lib/catalog';

export default function Home(catalog: HomeCatalog) {
    return (
        <>
            <Head>
                <meta name="keywords" content={site.keywords}/>
                <link rel="icon" href={site.favicon}/>
                <title>{site.seo.title}</title>
            </Head>
            <App resources={catalog.resources} ads={catalog.ads}/>
        </>
    )
}

export const getStaticProps: GetStaticProps<HomeCatalog> = async () => {
    const catalog = await loadHomeCatalog('zh-CN');
    return { props: catalog, revalidate: 300 };
};
