// Stream Deck plugin for ipcheck: a summary key and a key per routing group,
// fed by the agent's SSE stream. A key press runs a check immediately.
import streamDeck, { SingletonAction } from "@elgato/streamdeck";

import { AgentFeed } from "./feed.js";
import { routeKey, summaryKey, useFlags } from "./keys.js";

const DEFAULT_URL = "http://127.0.0.1:8787";

useFlags(new URL("../imgs/flags/", import.meta.url)); // relative to bin/plugin.js

const feed = new AgentFeed(renderAll);
const settings = new Map(); // action id → its settings
let blinkPhase = false;

class KeyAction extends SingletonAction {
	onWillAppear(ev) {
		settings.set(ev.action.id, ev.payload.settings);
		this.render(ev.action);
	}

	onWillDisappear(ev) {
		settings.delete(ev.action.id);
	}

	onDidReceiveSettings(ev) {
		settings.set(ev.action.id, ev.payload.settings);
		this.render(ev.action);
	}

	async onKeyDown(ev) {
		try {
			await feed.checkNow();
			await ev.action.showOk();
		} catch (err) {
			streamDeck.logger.warn(`check: ${err}`);
			await ev.action.showAlert();
		}
	}

	renderAll() {
		for (const a of this.actions) this.render(a);
	}
}

class SummaryAction extends KeyAction {
	manifestId = "com.ipcheck.streamdeck.summary";

	render(a) {
		const blink = settings.get(a.id)?.blink !== false;
		a.setImage(summaryKey(feed, blink && blinkPhase));
	}
}

class RouteAction extends KeyAction {
	manifestId = "com.ipcheck.streamdeck.route";

	render(a) {
		a.setImage(routeKey(feed, settings.get(a.id)?.group));
	}
}

const summary = new SummaryAction();
const route = new RouteAction();

function renderAll() {
	summary.renderAll();
	route.renderAll();
}

// Blink the summary key while there are problems; re-render when the state turns stale.
let wasStale = false;
setInterval(() => {
	const stale = feed.stale();
	if (stale !== wasStale) {
		wasStale = stale;
		renderAll();
	}
	const problem = feed.online && !stale && feed.state?.problems > 0;
	if (problem || blinkPhase) {
		blinkPhase = problem && !blinkPhase;
		summary.renderAll();
	}
}, 800);

streamDeck.settings.onDidReceiveGlobalSettings((ev) => feed.setURL(ev.settings.url || DEFAULT_URL));

streamDeck.actions.registerAction(summary);
streamDeck.actions.registerAction(route);
await streamDeck.connect();
feed.setURL((await streamDeck.settings.getGlobalSettings()).url || DEFAULT_URL);
