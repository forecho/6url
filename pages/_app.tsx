import '../styles/globals.css'
import type { AppProps } from 'next/app'
import {createTheme, NextUIProvider} from '@nextui-org/react';
import { DefaultSeo } from 'next-seo';
import {site} from "../data";
import { Analytics } from '@vercel/analytics/react';
import { SSRProvider } from '@react-aria/ssr';

export default function App({ Component, pageProps }: AppProps) {
    const darkTheme = createTheme({
        type: 'dark',
    });
  return (
      <SSRProvider>
          <NextUIProvider theme={darkTheme}>
              <DefaultSeo {...site.seo} />
              <Component {...pageProps} />
              <Analytics />
          </NextUIProvider>
      </SSRProvider>
  );
}
