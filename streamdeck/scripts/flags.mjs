// Renders flag-icons SVGs into small PNGs (imgs/flags/<cc>.png): the Stream Deck
// SVG renderer handles embedded PNG reliably, while emoji flags and complex SVG may not render.
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { Resvg } from "@resvg/resvg-js";

const src = "node_modules/flag-icons/flags/4x3";
const out = "com.ipcheck.streamdeck.sdPlugin/imgs/flags";
mkdirSync(out, { recursive: true });
for (const f of readdirSync(src).filter((f) => /^[a-z]{2}\.svg$/.test(f))) {
	const png = new Resvg(readFileSync(`${src}/${f}`), { fitTo: { mode: "width", value: 96 } }).render().asPng();
	writeFileSync(`${out}/${f.replace(".svg", ".png")}`, png);
}
