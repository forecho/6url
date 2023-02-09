import '../styles/globals.css'
import type { AppProps } from 'next/app'
import {createTheme, NextUIProvider} from '@nextui-org/react';
import { DefaultSeo } from 'next-seo';
import {site} from "./api";
import { Analytics } from '@vercel/analytics/react';

export default function App({ Component, pageProps }: AppProps) {
    const darkTheme = createTheme({
        type: 'dark',
    });
  return (
      <NextUIProvider theme={darkTheme}>
          <DefaultSeo {...site.seo} />
        <Component {...pageProps} />
        <Analytics /> 
      </NextUIProvider>
  );
}
