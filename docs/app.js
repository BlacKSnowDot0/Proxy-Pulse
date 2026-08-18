const DASHBOARD_URL = "data/dashboard.json";
const PROXIES_URL = "data/proxies.json";
const RAW_BASE = "https://raw.githubusercontent.com/BlacKSnowDot0/Proxy-Pulse/main/";

const state = {
  dashboard: null,
  proxies: [],
  range: 30,
  filter: "all",
  country: "all",
  query: "",
  sort: { key: "latency", dir: "asc" },
  showAll: false,
};

const chartColors = {
  validated: "#d4b06a",
  checked: "#7bc6a4",
  http: "#7bc6a4",
  socks4: "#e0a458",
  socks5: "#d97866",
};

const tooltip = document.getElementById("tooltip");

boot();

async function boot() {
  try {
    const [dashboard, proxyDataset] = await Promise.all([
      fetchJSON(DASHBOARD_URL),
      fetchJSON(PROXIES_URL),
    ]);
    state.dashboard = dashboard;
    state.proxies = Array.isArray(proxyDataset.proxies) ? proxyDataset.proxies : [];
    renderAll();
    document.body.classList.add("is-loaded");
  } catch (error) {
    renderError(error);
  }
}

async function fetchJSON(url) {
  const response = await fetch(url, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status} for ${url}`);
  }
  return response.json();
}

function renderAll() {
  renderTopbar();
  renderLive();
  renderProtocolBars();
  renderStatus();
  renderDownloads();
  renderCountryFilter();
  renderProtoFilter();
  renderProxyTable();
  renderRangeControls();
  renderCharts();
  renderRecentRuns();
}

/* ---------- topbar / status ---------- */

function renderTopbar() {
  const summary = state.dashboard.summary || {};
  const badge = document.getElementById("status-badge");
  const status = summary.status || "unknown";
  badge.textContent = humanize(status);
  badge.dataset.state = statusTone(status);

  const updated = document.getElementById("last-updated");
  const generated = summary.last_generated ? `updated ${formatTimestamp(summary.last_generated)}` : "no runs yet";
  const success = summary.last_success_at ? ` · last full success ${formatTimestamp(summary.last_success_at)}` : "";
  updated.textContent = generated + success;
}

function statusTone(status) {
  if (status === "success") return "ok";
  if (status === "success_with_errors" || status === "timeout") return "warn";
  if (status === "no_valid_proxies" || status === "error") return "err";
  return "loading";
}

function humanize(value) {
  return String(value || "unknown").replaceAll("_", " ");
}

/* ---------- live card ---------- */

function renderLive() {
  const summary = state.dashboard.summary || {};
  const counts = summary.current_output_counts || {};
  const history = sortedHistory();
  const latest = history.at(-1) || {};
  const previous = history.at(-2) || null;

  document.getElementById("live-count").textContent = formatNumber(counts.all ?? latest.validated ?? 0);

  const deltaNote = document.getElementById("live-delta");
  if (previous) {
    const deltaText = deltaTextFor(latest.validated, previous.validated, "vs previous run");
    deltaNote.textContent = deltaText.text;
    deltaNote.className = `card-note ${deltaText.direction}`;
  } else {
    deltaNote.textContent = "latest published count";
    deltaNote.className = "card-note";
  }

  const spark = document.getElementById("live-spark");
  const window = history.slice(-30).map((entry) => entry.validated || 0);
  const max = Math.max(...window, 1);
  spark.innerHTML = window
    .map((value) => {
      const height = Math.max(8, Math.round((value / max) * 100));
      const opacity = (0.3 + (0.7 * value) / max).toFixed(2);
      return `<span style="height:${height}%;opacity:${opacity}"></span>`;
    })
    .join("");
}

function deltaTextFor(current, previous, suffix) {
  const delta = Number(current || 0) - Number(previous || 0);
  if (delta === 0) return { text: `flat ${suffix}`, direction: "" };
  if (delta > 0) return { text: `+${formatNumber(delta)} ${suffix}`, direction: "is-up" };
  return { text: `−${formatNumber(Math.abs(delta))} ${suffix}`, direction: "is-down" };
}

/* ---------- protocol bars ---------- */

function renderProtocolBars() {
  const summary = state.dashboard.summary || {};
  const counts = summary.current_output_counts || {};
  const rows = [
    { key: "http", label: "HTTP", value: counts.http || 0 },
    { key: "https", label: "HTTPS", value: counts.https || 0 },
    { key: "socks4", label: "SOCKS4", value: counts.socks4 || 0 },
    { key: "socks5", label: "SOCKS5", value: counts.socks5 || 0 },
  ];
  const max = Math.max(...rows.map((row) => row.value), 1);

  document.getElementById("protocol-bars").innerHTML = rows
    .map(
      (row) => `
        <div class="proto-row">
          <span class="proto-name">${row.label}</span>
          <span class="proto-track"><span class="proto-fill" data-proto="${row.key}" style="width:${Math.max(2, Math.round((row.value / max) * 100))}%"></span></span>
          <span class="proto-count">${formatNumber(row.value)}</span>
        </div>
      `
    )
    .join("");
}

function renderStatus() {
  const summary = state.dashboard.summary || {};
  const status = summary.status || "unknown";
  const line = document.getElementById("status-text");
  const wrap = line.closest(".status-line");
  line.textContent = status === "success" ? "healthy" : humanize(status);
  wrap.classList.remove("is-ok", "is-warn", "is-err");
  const tone = statusTone(status);
  if (tone === "ok") wrap.classList.add("is-ok");
  if (tone === "warn") wrap.classList.add("is-warn");
  if (tone === "err") wrap.classList.add("is-err");

  const meta = document.getElementById("status-meta");
  meta.textContent = `${formatNumber(summary.runs_total || 0)} runs · ${formatCompact(summary.proxies_checked_total || 0)} probes · GitHub Actions`;
}

/* ---------- downloads ---------- */

function renderDownloads() {
  const summary = state.dashboard.summary || {};
  const counts = summary.current_output_counts || {};
  const files = [
    { name: "all.txt", label: "all", count: counts.all || 0, color: "#d4b06a" },
    { name: "http.txt", label: "http", count: counts.http || 0, color: "#7bc6a4" },
    { name: "https.txt", label: "https", count: counts.https || 0, color: "#e9cb8e" },
    { name: "socks4.txt", label: "socks4", count: counts.socks4 || 0, color: "#e0a458" },
    { name: "socks5.txt", label: "socks5", count: counts.socks5 || 0, color: "#d97866" },
  ];

  document.getElementById("download-row").innerHTML = files
    .map(
      (file) => `
        <a class="download" href="${RAW_BASE}${file.name}" rel="noopener">
          <span class="dl-dot" style="background:${file.color}"></span>
          <span class="dl-name">${file.label}.txt</span>
          <span class="dl-count">${formatNumber(file.count)}</span>
        </a>
      `
    )
    .join("");
}

/* ---------- proxy explorer ---------- */

const PROTO_FILTERS = [
  { key: "all", label: "All" },
  { key: "http", label: "HTTP" },
  { key: "https", label: "HTTPS" },
  { key: "socks4", label: "SOCKS4" },
  { key: "socks5", label: "SOCKS5" },
];

function renderProtoFilter() {
  const container = document.getElementById("proto-filter");
  container.innerHTML = PROTO_FILTERS
    .map(
      (filter) =>
        `<button class="fchip" type="button" data-filter="${filter.key}" aria-pressed="${state.filter === filter.key}">${filter.label}</button>`
    )
    .join("");

  container.querySelectorAll("[data-filter]").forEach((button) => {
    button.addEventListener("click", () => {
      state.filter = button.dataset.filter;
      state.showAll = false;
      renderProtoFilter();
      renderProxyTable();
    });
  });
}

function renderCountryFilter() {
  const select = document.getElementById("country-filter");
  const countries = new Map();
  for (const proxy of state.proxies) {
    const code = proxy.country_code || "??";
    if (!countries.has(code)) countries.set(code, proxy.country_name || "");
  }
  const options = ["all", ...[...countries.keys()].sort()];
  select.innerHTML = options
    .map((code) => {
      if (code === "all") return `<option value="all">all countries</option>`;
      const name = countries.get(code);
      return `<option value="${escapeHtml(code)}">${escapeHtml(code)}${name ? ` — ${escapeHtml(name)}` : ""}</option>`;
    })
    .join("");
  select.value = state.country;

  select.addEventListener("change", () => {
    state.country = select.value;
    state.showAll = false;
    renderProxyTable();
  });

  const search = document.getElementById("proxy-search");
  search.addEventListener("input", () => {
    state.query = search.value.trim().toLowerCase();
    state.showAll = false;
    renderProxyTable();
  });
}

const sortAccessors = {
  address: (proxy) => proxy.address || `${proxy.host}:${proxy.port}`,
  country: (proxy) => proxy.country_code || "zz",
  latency: (proxy) => (proxy.latency_ms > 0 ? proxy.latency_ms : Number.MAX_SAFE_INTEGER),
  uptime: (proxy) => proxy.uptime_pct || 0,
};

function filteredProxies() {
  const query = state.query;
  return state.proxies.filter((proxy) => {
    if (state.filter === "https") {
      if (!(proxy.protocol === "http" && proxy.https_ok)) return false;
    } else if (state.filter !== "all" && proxy.protocol !== state.filter) {
      return false;
    }
    if (state.country !== "all" && (proxy.country_code || "??") !== state.country) return false;
    if (query) {
      const haystack = `${proxy.host}:${proxy.port} ${proxy.country_code || ""} ${proxy.country_name || ""} ${proxy.org || ""} ${proxy.asn || ""}`.toLowerCase();
      if (!haystack.includes(query)) return false;
    }
    return true;
  });
}

function renderProxyTable() {
  const rows = filteredProxies();
  const { key, dir } = state.sort;
  const accessor = sortAccessors[key] || sortAccessors.latency;
  const sorted = [...rows].sort((a, b) => {
    const av = accessor(a);
    const bv = accessor(b);
    if (av === bv) return 0;
    const order = av > bv ? 1 : -1;
    return dir === "asc" ? order : -order;
  });

  document.querySelectorAll(".proxy-table th[data-sort]").forEach((th) => {
    if (th.dataset.sort === key) {
      th.setAttribute("aria-sort", dir === "asc" ? "ascending" : "descending");
    } else {
      th.removeAttribute("aria-sort");
    }
  });

  const limit = state.showAll ? sorted.length : 150;
  const visible = sorted.slice(0, limit);

  const body = document.getElementById("proxy-rows");
  if (!visible.length) {
    body.innerHTML = `<tr><td colspan="8"><div class="empty-state">No proxies match the current filters.</div></td></tr>`;
  } else {
    body.innerHTML = visible.map(proxyRow).join("");
    body.querySelectorAll("[data-copy]").forEach(bindCopyButton);
  }

  document.getElementById("proxy-count").textContent =
    sorted.length === state.proxies.length
      ? `${formatNumber(sorted.length)} proxies`
      : `${formatNumber(sorted.length)} of ${formatNumber(state.proxies.length)} proxies`;

  const showAllButton = document.getElementById("show-all");
  showAllButton.hidden = sorted.length <= limit;
  showAllButton.textContent = `show all ${formatNumber(sorted.length)}`;
}

function proxyRow(proxy) {
  const address = `${proxy.host}:${proxy.port}`;
  const uri = `${proxy.protocol}://${address}`;
  const latency = proxy.latency_ms || 0;
  const latencyClass = latency > 0 ? (latency < 800 ? "lat-good" : latency < 1500 ? "lat-mid" : "lat-bad") : "";
  const uptime = proxy.uptime_pct || 0;
  return `
    <tr>
      <td>${escapeHtml(address)}</td>
      <td><span class="badge" data-proto="${escapeHtml(proxy.protocol)}">${escapeHtml(proxy.protocol.toUpperCase())}</span></td>
      <td><span class="cc">${escapeHtml(proxy.country_code || "??")}</span>${proxy.country_name ? ` <span class="card-note">${escapeHtml(proxy.country_name)}</span>` : ""}</td>
      <td class="card-note" style="max-width:260px;overflow:hidden;text-overflow:ellipsis">${escapeHtml(proxy.org || (proxy.asn ? `AS${proxy.asn}` : "—"))}</td>
      <td class="th-num-target"><span class="lat ${latencyClass}">${latency ? `${formatNumber(latency)} ms` : "—"}</span></td>
      <td class="th-num-target">${
        uptime
          ? `<span class="uptime-cell"><span class="uptime-bar"><i style="width:${Math.max(6, uptime)}%"></i></span>${uptime}%</span>`
          : "—"
      }</td>
      <td>${proxy.protocol === "http" ? (proxy.https_ok ? `<span class="flag-yes">✓</span>` : `<span class="flag-no">—</span>`) : ""}</td>
      <td><button class="copy-mini" type="button" data-copy="${escapeAttribute(uri)}" aria-label="Copy ${escapeHtml(uri)}">copy</button></td>
    </tr>
  `;
}

document.addEventListener("click", (event) => {
  const th = event.target.closest("th[data-sort]");
  if (!th) return;
  const key = th.dataset.sort;
  if (state.sort.key === key) {
    state.sort.dir = state.sort.dir === "asc" ? "desc" : "asc";
  } else {
    state.sort = { key, dir: key === "address" || key === "country" ? "asc" : "asc" };
  }
  renderProxyTable();
});

document.getElementById("show-all").addEventListener("click", () => {
  state.showAll = true;
  renderProxyTable();
});

document.getElementById("copy-filtered").addEventListener("click", (event) => {
  const lines = filteredProxies().map((proxy) => `${proxy.protocol}://${proxy.host}:${proxy.port}`);
  if (!lines.length) return;
  copyText(lines.join("\n"), event.currentTarget);
});

function bindCopyButton(button) {
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    copyText(button.dataset.copy, button);
  });
}

async function copyText(text, button) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    document.execCommand("copy");
    area.remove();
  }
  if (!button) return;
  const original = button.textContent;
  button.dataset.state = "done";
  button.textContent = "copied";
  setTimeout(() => {
    button.dataset.state = "idle";
    button.textContent = original;
  }, 1400);
}

/* ---------- charts ---------- */

function sortedHistory() {
  const history = Array.isArray(state.dashboard.history) ? [...state.dashboard.history] : [];
  history.sort((a, b) => a.finished_at.localeCompare(b.finished_at));
  return history;
}

function renderRangeControls() {
  const total = sortedHistory().length;
  const options = [7, 30, 90, 180];
  const controls = document.getElementById("range-controls");
  controls.innerHTML = options
    .map((value) => {
      const active = state.range === value ? " is-active" : "";
      const label = total < value ? `${value} max` : `${value} runs`;
      return `<button class="fchip${active}" type="button" data-range="${value}" aria-pressed="${state.range === value}">${label}</button>`;
    })
    .join("");

  controls.querySelectorAll("[data-range]").forEach((button) => {
    button.addEventListener("click", () => {
      state.range = Number(button.dataset.range);
      renderRangeControls();
      renderCharts();
    });
  });
}

function currentWindow() {
  const history = sortedHistory();
  return history.slice(-Math.min(state.range, history.length || state.range));
}

function renderCharts() {
  const history = currentWindow();

  renderChartMetrics("chart-validated-meta", [
    `Latest ${formatNumber(history.at(-1)?.validated || 0)}`,
    `Peak ${formatNumber(Math.max(...history.map((e) => e.validated), 0))}`,
    `Avg ${formatNumber(Math.round(average(history.map((e) => e.validated))))}`,
  ]);
  renderLineChart(document.getElementById("chart-validated"), {
    history,
    series: [
      { key: "validated", label: "Validated", color: chartColors.validated, formatter: (e) => `${formatNumber(e.validated)} validated`, fillArea: true },
    ],
  });

  renderChartMetrics("chart-checked-meta", [
    `Checked ${formatNumber(history.at(-1)?.proxies_checked || 0)}`,
    `Yield ${rate(history.at(-1)?.validated || 0, history.at(-1)?.proxies_checked || 0)}`,
  ]);
  renderLineChart(document.getElementById("chart-checked"), {
    history,
    series: [
      { key: "proxies_checked", label: "Checked", color: chartColors.checked, formatter: (e) => `${formatNumber(e.proxies_checked)} checked` },
      { key: "validated", label: "Validated", color: chartColors.validated, formatter: (e) => `${formatNumber(e.validated)} validated` },
    ],
  });

  renderChartMetrics("chart-protocols-meta", [
    `HTTP ${formatNumber(valueAt(history.at(-1) || {}, "output_counts.http"))}`,
    `S4 ${formatNumber(valueAt(history.at(-1) || {}, "output_counts.socks4"))}`,
    `S5 ${formatNumber(valueAt(history.at(-1) || {}, "output_counts.socks5"))}`,
  ]);
  renderLineChart(document.getElementById("chart-protocols"), {
    history,
    series: [
      { key: "output_counts.http", label: "HTTP", color: chartColors.http, formatter: (e) => `${formatNumber(valueAt(e, "output_counts.http"))} HTTP` },
      { key: "output_counts.socks4", label: "SOCKS4", color: chartColors.socks4, formatter: (e) => `${formatNumber(valueAt(e, "output_counts.socks4"))} SOCKS4` },
      { key: "output_counts.socks5", label: "SOCKS5", color: chartColors.socks5, formatter: (e) => `${formatNumber(valueAt(e, "output_counts.socks5"))} SOCKS5` },
    ],
  });
}

function renderChartMetrics(targetId, items) {
  document.getElementById(targetId).innerHTML = items
    .map((item) => `<span class="chart-metric">${escapeHtml(item)}</span>`)
    .join("");
}

function renderLineChart(target, config) {
  const { history, series } = config;
  if (!history.length) {
    target.innerHTML = `<div class="empty-state">No history available yet.</div>`;
    return;
  }

  const width = 720;
  const height = 260;
  const margin = { top: 18, right: 14, bottom: 34, left: 46 };
  const innerWidth = width - margin.left - margin.right;
  const innerHeight = height - margin.top - margin.bottom;
  const xStep = history.length === 1 ? 0 : innerWidth / (history.length - 1);
  const values = history.flatMap((entry) => series.map((item) => valueAt(entry, item.key)));
  const maxValue = Math.max(...values, 1);
  const gridValues = [0, maxValue / 2, maxValue];

  const defs = series
    .filter((item) => item.fillArea)
    .map(
      (item, index) => `
        <linearGradient id="fill-${target.id}-${index}" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="${item.color}" stop-opacity="0.3"></stop>
          <stop offset="100%" stop-color="${item.color}" stop-opacity="0.02"></stop>
        </linearGradient>
      `
    )
    .join("");

  const lines = series
    .map((item, seriesIndex) => {
      const points = history.map((entry, index) => {
        const x = margin.left + xStep * index;
        const y = margin.top + innerHeight - (valueAt(entry, item.key) / maxValue) * innerHeight;
        return { x, y, entry };
      });

      const path = points
        .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`)
        .join(" ");

      const areaPath = item.fillArea
        ? `${path} L ${points[points.length - 1].x.toFixed(2)} ${(margin.top + innerHeight).toFixed(2)} L ${points[0].x.toFixed(2)} ${(margin.top + innerHeight).toFixed(2)} Z`
        : "";

      const circles = points
        .map((point) => {
          const detail = `${item.label}: ${item.formatter(point.entry)}\n${formatTimestamp(point.entry.finished_at)}`;
          return `<circle class="data-point" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="4" fill="${item.color}" data-tooltip="${escapeAttribute(detail)}"><title>${escapeHtml(detail)}</title></circle>`;
        })
        .join("");

      return `
        <g>
          ${item.fillArea ? `<path d="${areaPath}" fill="url(#fill-${target.id}-${seriesIndex})"></path>` : ""}
          <path d="${path}" fill="none" stroke="${item.color}" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"></path>
          ${circles}
        </g>
      `;
    })
    .join("");

  const grid = gridValues
    .map((value) => {
      const y = margin.top + innerHeight - (value / maxValue) * innerHeight;
      return `
        <g>
          <line class="grid-line" x1="${margin.left}" y1="${y.toFixed(2)}" x2="${width - margin.right}" y2="${y.toFixed(2)}"></line>
          <text class="axis-label" x="6" y="${(y + 4).toFixed(2)}">${formatCompact(value)}</text>
        </g>
      `;
    })
    .join("");

  const bands = history
    .map((entry, index) => {
      const x = margin.left + xStep * index;
      const bandWidth = history.length === 1 ? innerWidth : Math.max(xStep, 16);
      const left = Math.max(margin.left, x - bandWidth / 2);
      const detail = `${formatTimestamp(entry.finished_at)}\nValidated ${formatNumber(entry.validated)}\nChecked ${formatNumber(entry.proxies_checked)}`;
      return `<rect class="hover-band data-point" x="${left.toFixed(2)}" y="${margin.top}" width="${bandWidth.toFixed(2)}" height="${innerHeight}" data-tooltip="${escapeAttribute(detail)}" fill="transparent"></rect>`;
    })
    .join("");

  const xLabels = labelPositions(history, margin.left, xStep)
    .map((label) => `<text class="axis-label" x="${label.x.toFixed(2)}" y="${height - 10}" text-anchor="${label.anchor || "start"}">${escapeHtml(label.text)}</text>`)
    .join("");

  const legend = series
    .map((item, index) => {
      const x = margin.left + index * 108;
      return `
        <g transform="translate(${x}, 12)">
          <circle cx="0" cy="0" r="4.5" fill="${item.color}"></circle>
          <text class="axis-label" x="11" y="4">${escapeHtml(item.label)}</text>
        </g>
      `;
    })
    .join("");

  target.innerHTML = `
    <svg viewBox="0 0 ${width} ${height}" role="img" aria-label="${escapeHtml(series.map((item) => item.label).join(", "))} chart">
      <defs>${defs}</defs>
      ${grid}
      ${bands}
      ${lines}
      ${legend}
      ${xLabels}
    </svg>
  `;

  bindTooltip(target);
}

function labelPositions(history, startX, xStep) {
  if (history.length === 1) {
    return [{ x: startX, text: shortDate(history[0].finished_at), anchor: "start" }];
  }
  const lastIndex = history.length - 1;
  return [
    { x: startX, text: shortDate(history[0].finished_at), anchor: "start" },
    { x: startX + (xStep * lastIndex) / 2, text: shortDate(history[Math.floor(lastIndex / 2)].finished_at), anchor: "middle" },
    { x: startX + xStep * lastIndex, text: shortDate(history[lastIndex].finished_at), anchor: "end" },
  ];
}

/* ---------- recent runs ---------- */

function renderRecentRuns() {
  const rows = sortedHistory().reverse().slice(0, 12);
  document.getElementById("recent-runs").innerHTML = rows
    .map(
      (entry) => `
        <tr>
          <td class="card-note">${escapeHtml(formatTimestamp(entry.finished_at))}</td>
          <td><span class="pill" data-state="${statusTone(entry.status)}">${escapeHtml(humanize(entry.status))}</span></td>
          <td class="th-num-target">${escapeHtml(formatNumber(entry.validated))}</td>
          <td class="th-num-target">${escapeHtml(formatNumber(entry.proxies_checked))}</td>
          <td class="th-num-target">${escapeHtml(formatNumber(entry.requests_made))}</td>
        </tr>
      `
    )
    .join("");
}

/* ---------- tooltip ---------- */

function bindTooltip(target) {
  target.querySelectorAll(".data-point").forEach((point) => {
    point.addEventListener("mouseenter", showTooltip);
    point.addEventListener("mousemove", showTooltip);
    point.addEventListener("mouseleave", hideTooltip);
  });
}

function showTooltip(event) {
  tooltip.hidden = false;
  tooltip.innerHTML = escapeHtml(event.currentTarget.dataset.tooltip).replaceAll("\n", "<br>");
  const offset = 14;
  const maxLeft = window.innerWidth - tooltip.offsetWidth - 12;
  const maxTop = window.innerHeight - tooltip.offsetHeight - 12;
  const nextLeft = Math.min(event.clientX + offset, Math.max(12, maxLeft));
  const nextTop = Math.min(event.clientY + offset, Math.max(12, maxTop));
  tooltip.style.left = `${nextLeft}px`;
  tooltip.style.top = `${nextTop}px`;
}

function hideTooltip() {
  tooltip.hidden = true;
}

/* ---------- error + helpers ---------- */

function renderError(error) {
  const badge = document.getElementById("status-badge");
  badge.textContent = "Unavailable";
  badge.dataset.state = "err";
  document.getElementById("last-updated").textContent = `failed to load datasets: ${error.message}`;
  document.getElementById("live-delta").textContent = "data unavailable";
  document.getElementById("proxy-rows").innerHTML = `<tr><td colspan="8"><div class="empty-state">Dashboard data could not be loaded — try refreshing.</div></td></tr>`;
  document.getElementById("recent-runs").innerHTML = `<tr><td colspan="5"><div class="empty-state">No run data available.</div></td></tr>`;
}

function valueAt(entry, key) {
  return key.split(".").reduce((value, part) => (value && value[part] !== undefined ? value[part] : 0), entry) || 0;
}

function average(values) {
  return values.length ? values.reduce((sum, value) => sum + Number(value || 0), 0) / values.length : 0;
}

function rate(part, total) {
  return total ? `${Math.round((100 * part) / total)}%` : "0%";
}

function shortDate(value) {
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(new Date(value));
}

function formatTimestamp(value) {
  if (!value) return "n/a";
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value || 0));
}

function formatCompact(value) {
  return new Intl.NumberFormat(undefined, { notation: "compact", maximumFractionDigits: 1 }).format(value);
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function escapeAttribute(value) {
  return escapeHtml(value).replaceAll("`", "&#96;");
}
