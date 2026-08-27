import type { Metadata } from "next";
import { Geist, JetBrains_Mono } from "next/font/google";
import { I18nProvider } from "./i18n/client";
import { getRequestLocale, getServerTranslator } from "./i18n/server";
import { directionForLocale } from "./i18n/settings";
import "./globals.css";

const geist = Geist({ variable: "--font-geist", subsets: ["latin"] });
const jetBrainsMono = JetBrains_Mono({ variable: "--font-jetbrains-mono", subsets: ["latin"] });

export async function generateMetadata(): Promise<Metadata> {
  const locale = await getRequestLocale();
  const t = await getServerTranslator(locale);
  return {
    title: t("metadata.title"),
    description: t("metadata.description"),
    icons: { icon: "/favicon.svg", shortcut: "/favicon.svg" },
  };
}

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const locale = await getRequestLocale();
  return (
    <html
      lang={locale}
      dir={directionForLocale(locale)}
      className={`${geist.variable} ${jetBrainsMono.variable}`}
      suppressHydrationWarning
    >
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `(function(){try{var t=localStorage.getItem("dokosoko-theme");var d=t==="dark"||t!=="light"&&matchMedia("(prefers-color-scheme: dark)").matches;document.documentElement.dataset.theme=d?"dark":"light"}catch(e){document.documentElement.dataset.theme="light"}})()`,
          }}
        />
      </head>
      <body><I18nProvider locale={locale}>{children}</I18nProvider></body>
    </html>
  );
}
