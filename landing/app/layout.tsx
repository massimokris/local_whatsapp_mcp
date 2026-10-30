import type { Metadata } from "next";
import { fontClassName } from "./fonts";
import { DESCRIPTION } from "./site";
import "./globals.css";

const deploymentHost = process.env.VERCEL_PROJECT_PRODUCTION_URL || process.env.VERCEL_URL;
const siteOrigin = process.env.SITE_URL ||
  (deploymentHost ? `https://${deploymentHost}` : "http://localhost:3000");
const setupPreview = {
  url: "/setup.gif",
  width: 1000,
  height: 860,
  alt: "LocalWhatsAppMCP setup: link WhatsApp, choose chats, and connect your agent.",
};

export const metadata: Metadata = {
  metadataBase: new URL(siteOrigin),
  title: "LocalWhatsAppMCP",
  description: DESCRIPTION,
  openGraph: {
    title: "LocalWhatsAppMCP",
    description: DESCRIPTION,
    type: "website",
    images: [{ ...setupPreview, type: "image/gif" }],
  },
  twitter: {
    card: "summary_large_image",
    title: "LocalWhatsAppMCP",
    description: DESCRIPTION,
    images: [setupPreview],
  },
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  // Browser extensions can add attributes to <html> before hydration.
  return (
    <html lang="en" className={fontClassName} suppressHydrationWarning>
      <body>{children}</body>
    </html>
  );
}
