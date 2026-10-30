import { DownloadForm } from "./DownloadForm";
import { DESCRIPTION, REPOSITORY_URL } from "./site";

export default function Home() {
  return (
    <div className="page">
      <header className="site-header">
        <a href={REPOSITORY_URL}>repo</a>
      </header>

      <main>
        <h1>LocalWhatsAppMCP</h1>
        <div className="copy">
          <p>{DESCRIPTION}</p>
          <p>
            scan a qr code, choose which chats your agent can access, and let it
            read messages, send replies and check for updates
          </p>
          <p>
            runs locally and privately on your mac.
            <br />
            open-source.
          </p>
        </div>

        <DownloadForm />

        <figure className="setup-preview">
          <img
            src="/setup.gif"
            width="1000"
            height="860"
            alt="Sample setup walkthrough: scan the WhatsApp QR code, choose chats, and copy instructions to connect your agent."
          />
        </figure>
      </main>
    </div>
  );
}
