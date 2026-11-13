// Key images: 144×144 SVG with a status-colored background and three text lines.
import { readFileSync } from "node:fs";

const BG = {
	ok: "#1e7e34",
	problem: "#c62828",
	problemDim: "#4a0d0d",
	dns: "#d35400",
	error: "#8b1a1a",
	stale: "#7a5a00",
	idle: "#3a3a3a",
};

const PROBLEM_TEXT = {
	mismatch: "не тот выход",
	unknown_exit: "чужой IP",
	dns_bypass: "мимо podkop",
	error: "нет ответа",
};

// summaryKey draws the overall state: green check, red problem count, or grey when unknown.
export function summaryKey(feed, blinkPhase) {
	const s = feed.state;
	if (!feed.online) return key(BG.idle, "IP CHECK", "?", "нет агента");
	if (!s?.results?.length) return key(BG.idle, "IP CHECK", "…", "проверка");
	if (feed.stale()) return key(BG.stale, "IP CHECK", "!", "устарело");
	if (s.ok) return key(BG.ok, "МАРШРУТЫ", CHECK, `${s.results.length}/${s.results.length} ok`);
	const bad = s.results.filter((r) => r.status !== "ok").map((r) => r.group);
	return key(blinkPhase ? BG.problemDim : BG.problem, "ПРОБЛЕМ", String(s.problems), bad.join(", "));
}

// routeKey draws one routing group: actual exit country, colored by status.
export function routeKey(feed, group) {
	if (!group) return key(BG.idle, "IP CHECK", "?", "выбери группу", 17);
	const name = group.toUpperCase();
	if (!feed.online) return key(BG.idle, name, "?", "нет агента");
	const r = feed.state?.results?.find((r) => r.group === group);
	if (!r) return key(BG.idle, name, "…", feed.state?.results?.length ? "нет группы" : "проверка");
	if (feed.stale()) return key(BG.stale, name, "!", "устарело");

	switch (r.status) {
		case "ok":
			return key(BG.ok, name, flagOr(r.country, "OK"), r.exit === group ? `${r.latency_ms} ms` : r.exit);
		case "mismatch":
			return key(BG.problem, name, flagOr(r.country, "?"), `ждали ${r.expect_country || r.expect}`);
		case "unknown_exit":
			return key(BG.problem, name, "??", r.ip, 17);
		case "dns_bypass":
			return key(BG.dns, name, "DNS", PROBLEM_TEXT.dns_bypass);
		default:
			return key(BG.error, name, "ERR", PROBLEM_TEXT.error);
	}
}

const CHECK = Symbol("check");

let flagDir = null;
const flags = new Map(); // country code → PNG data URL, or null when there is no flag

// useFlags sets the directory with pre-rendered flags (<cc>.png, see scripts/flags.mjs).
export function useFlags(dir) {
	flagDir = dir;
	flags.clear();
}

// flagOr returns the flag of a country for the key center, or the fallback text without one.
function flagOr(cc, fallback) {
	if (!cc) return fallback;
	cc = cc.toLowerCase();
	if (!flags.has(cc)) {
		let url = null;
		try {
			url = "data:image/png;base64," + readFileSync(new URL(`${cc}.png`, flagDir)).toString("base64");
		} catch {
			// unknown code or flags not built: show the code as text
		}
		flags.set(cc, url);
	}
	return flags.get(cc) ? { flag: flags.get(cc) } : cc.toUpperCase();
}

function key(bg, top, middle, bottom = "", bottomSize = 20) {
	let center;
	if (middle === CHECK) {
		center = `<path d="M44 74 l18 18 l38 -40" fill="none" stroke="#fff" stroke-width="13" stroke-linecap="round" stroke-linejoin="round"/>`;
	} else if (middle?.flag) {
		center = `<image x="30" y="44" width="84" height="63" href="${middle.flag}" xlink:href="${middle.flag}" preserveAspectRatio="none"/>
<rect x="30" y="44" width="84" height="63" fill="none" stroke="#fff" stroke-opacity="0.6" stroke-width="2"/>`;
	} else {
		center = text(72, 92, fit(middle, 4), middle.length > 3 ? 40 : 54);
	}
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="144" height="144" viewBox="0 0 144 144">
<rect width="144" height="144" fill="${bg}"/>
${text(72, 30, fit(top, 11), 19, 0.85)}
${center}
${text(72, 128, fit(bottom, Math.floor(260 / bottomSize)), bottomSize)}
</svg>`;
	return "data:image/svg+xml;base64," + Buffer.from(svg).toString("base64");
}

function text(x, y, s, size, opacity = 1) {
	if (!s) return "";
	return `<text x="${x}" y="${y}" font-family="Helvetica Neue, Helvetica, Arial, sans-serif" font-size="${size}" font-weight="bold" fill="#fff" fill-opacity="${opacity}" text-anchor="middle">${esc(s)}</text>`;
}

function fit(s, max) {
	s = String(s ?? "");
	return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

function esc(s) {
	return s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
}
