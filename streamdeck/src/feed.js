// AgentFeed follows the agent's SSE stream (/api/events) and keeps the latest state.
export class AgentFeed {
	url = "";
	state = null; // last agent.State, null until the first event
	online = false;
	#abort = null;
	#onChange;

	constructor(onChange) {
		this.#onChange = onChange;
	}

	setURL(url) {
		url = url.replace(/\/+$/, "");
		if (url === this.url) return;
		this.url = url;
		this.#abort?.abort();
		const abort = new AbortController();
		this.#abort = abort;
		this.#run(url, abort.signal);
	}

	async checkNow() {
		const res = await fetch(this.url + "/api/check", { method: "POST" });
		if (!res.ok) throw new Error(`HTTP ${res.status}`);
	}

	// stale reports that the last check is older than three intervals, e.g. the agent hangs.
	stale() {
		if (!this.state?.updated || !this.state.interval) return false;
		const age = Date.now() - Date.parse(this.state.updated);
		return age > 3 * parseDuration(this.state.interval) + 15_000;
	}

	async #run(url, signal) {
		while (!signal.aborted) {
			try {
				const res = await fetch(url + "/api/events", { signal });
				if (!res.ok) throw new Error(`HTTP ${res.status}`);
				const decoder = new TextDecoder();
				let buf = "";
				for await (const chunk of res.body) {
					buf += decoder.decode(chunk, { stream: true });
					let end;
					while ((end = buf.indexOf("\n\n")) >= 0) {
						const data = buf
							.slice(0, end)
							.split("\n")
							.filter((l) => l.startsWith("data:"))
							.map((l) => l.slice(5).trim())
							.join("\n");
						buf = buf.slice(end + 2);
						if (!data) continue;
						this.state = JSON.parse(data);
						this.online = true;
						this.#onChange();
					}
				}
			} catch {
				// reconnect below
			}
			if (signal.aborted) return;
			if (this.online) {
				this.online = false;
				this.#onChange();
			}
			await new Promise((r) => setTimeout(r, 5000));
		}
	}
}

// parseDuration parses a Go duration string such as "30s" or "1m30s" into milliseconds.
function parseDuration(s) {
	const units = { h: 3_600_000, m: 60_000, s: 1000, ms: 1 };
	let ms = 0;
	for (const [, n, u] of s.matchAll(/([\d.]+)(ms|h|m|s)/g)) ms += parseFloat(n) * units[u];
	return ms;
}
