import type { Metadata } from "next";
import { Geist, JetBrains_Mono } from "next/font/google";
import { I18nProvider } from "./i18n/client";
import { getServerTranslator } from "./i18n/server";
import { defaultLocale, directionForLocale } from "./i18n/settings";
import "./globals.css";

const geist = Geist({ variable: "--font-geist", subsets: ["latin"] });
const jetBrainsMono = JetBrains_Mono({ variable: "--font-jetbrains-mono", subsets: ["latin"] });

export async function generateMetadata(): Promise<Metadata> {
  const t = await getServerTranslator(defaultLocale);
  return {
    title: t("metadata.title"),
    description: t("metadata.description"),
    icons: { icon: "/favicon.svg", shortcut: "/favicon.svg" },
  };
}

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const locale = defaultLocale;
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
            __html: `(function(){try{var t=localStorage.getItem("dokosoko-theme");var p=t==="light"||t==="dark"||t==="system"?t:"system";var d=p==="dark"||p==="system"&&matchMedia("(prefers-color-scheme: dark)").matches;document.documentElement.dataset.themePreference=p;document.documentElement.dataset.theme=d?"dark":"light";document.documentElement.style.colorScheme=d?"dark":"light"}catch(e){document.documentElement.dataset.themePreference="system";document.documentElement.dataset.theme="light";document.documentElement.style.colorScheme="light"}})()`,
          }}
        />
      </head>
      <body><I18nProvider locale={locale}>{children}</I18nProvider></body>
    </html>
  );
}
