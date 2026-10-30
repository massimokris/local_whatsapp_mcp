import type { Metadata } from "next";
import { fontClassName } from "./fonts";
import { DESCRIPTION } from "./site";
import "./globals.css";

export const metadata: Metadata = {
  title: "LocalWhatsAppMCP",
  description: DESCRIPTION,
  openGraph: {
    title: "LocalWhatsAppMCP",
    description: DESCRIPTION,
    type: "website",
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
