const DATA_URL = "data/dashboard.json";

const state = {
  dashboard: null,
  range: 30,
};

const chartColors = {
  validated: "#d4b06a",
  checked: "#7bc6a4",
  http: "#f4e8cf",
  socks4: "#e0a458",
  socks5: "#d97866",
};

const tooltip = document.getElementById("tooltip");

boot();

async function boot() {
  try {
    const response = await fetch(DATA_URL, { cache: "no-store" });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    state.dashboard = await response.json();
    render();
    requestAnimationFrame(() => document.body.classList.add("is-loaded"));
  } catch (error) {
    renderError(error);
  }
}

function render() {
  const dashboard = state.dashboard;
  const history = Array.isArray(dashboard.history) ? [...dashboard.history] : [];
  history.sort((a, b) => a.finished_at.localeCompare(b.finished_at));

  const summary = dashboard.summary || {};
  const currentRange = history.slice(-Math.min(state.range, history.length || state.range));

  renderHeader(summary);
  renderHeroHighlights(summary, history);
  renderInsights(summary, history, currentRange);
  renderKpis(summary);
  renderRangeControls(history.length);
  renderValidatedChart(currentRange);
  renderCheckedChart(currentRange);
  renderProtocolChart(currentRange);
  renderRecentRuns(history);
}

function renderHeader(summary) {
  const badge = document.getElementById("status-badge");
  const lastUpdated = document.getElementById("last-updated");
  const status = summary.status || "unknown";

  badge.textContent = humanizeStatus(status);
  badge.className = `status-badge status-${status}`;
  lastUpdated.textContent = `Last updated ${formatTimestamp(summary.last_generated)}. Last successful refresh ${summary.last_success_at ? formatTimestamp(summary.last_success_at) : "n/a"}.`;
}

function renderHeroHighlights(summary, history) {
  const latest = history.at(-1) || {};
  const previous = history.at(-2) || null;
  const current = summary.current_output_counts || {};
  const cards = [
    {
      label: "Published total",
      value: formatNumber(current.all),
      note: previous ? deltaText(latest.validated, previous.validated, "validated vs prior run") : "Latest combined published count.",
    },
    {
      label: "Last run checked",
      value: formatNumber(latest.proxies_checked),
      note: "Latest validation pass volume.",
    },
    {
      label: "Last run requests",
      value: formatNumber(latest.requests_made),
      note: "Combined discovery and validation requests.",
    },
    {
      label: "History retained",
      value: formatNumber(history.length),
      note: "Newest-last rolling run window.",
    },
  ];

  document.getElementById("hero-highlights").innerHTML = cards
    .map(
      (card) => `
        <article class="mini-card">
          <p class="mini-label">${escapeHtml(card.label)}</p>
          <p class="mini-value">${escapeHtml(card.value)}</p>
          <p class="mini-note">${escapeHtml(card.note)}</p>
        </article>
      `
    )
    .join("");
}

function renderInsights(summary, history, currentRange) {
  const latest = history.at(-1) || {};
  const sourceCounts = summary.current_source_counts || {};
  const totalSources = Object.values(sourceCounts).reduce((sum, value) => sum + Number(value || 0), 0);
  const repositoryShare = totalSources ? Math.round((100 * (sourceCounts.repository || 0)) / totalSources) : 0;
  const gistShare = totalSources ? Math.round((100 * (sourceCounts.gist || 0)) / totalSources) : 0;
  const successRate = rate(currentRange.filter((entry) => isHealthyStatus(entry.status)).length, currentRange.length);

  document.getElementById("run-health").innerHTML = `
    <div class="insight-stack">
      <div class="insight-row">
        <span class="insight-label">Latest status</span>
        <span class="insight-value status-${latest.status || "unknown"}">${escapeHtml(humanizeStatus(latest.status || "unknown"))}</span>
      </div>
      <div class="insight-row">
        <span class="insight-label">Healthy run rate</span>
        <span class="insight-value">${escapeHtml(successRate)}</span>
      </div>
      <div class="insight-row">
        <span class="insight-label">Average validated in view</span>
        <span class="insight-value">${escapeHtml(formatNumber(Math.round(average(currentRange.map((entry) => entry.validated)))))}</span>
      </div>
    </div>
  `;

  document.getElementById("source-mix").innerHTML = `
    <div class="insight-stack">
      <div class="insight-row">
        <span class="insight-label">Repositories</span>
        <span class="insight-value">${escapeHtml(formatNumber(sourceCounts.repository || 0))} <span class="insight-copy">(${escapeHtml(String(repositoryShare))}%)</span></span>
      </div>
      <div class="insight-row">
        <span class="insight-label">Gists</span>
        <span class="insight-value">${escapeHtml(formatNumber(sourceCounts.gist || 0))} <span class="insight-copy">(${escapeHtml(String(gistShare))}%)</span></span>
      </div>
      <div class="insight-row">
        <span class="insight-label">Total active sources</span>
        <span class="insight-value">${escapeHtml(formatNumber(totalSources))}</span>
      </div>
    </div>
  `;
}

function renderKpis(summary) {
  const current = summary.current_output_counts || {};
  const cards = [
    {
      label: "Total runs",
      value: formatNumber(summary.runs_total),
      meta: "Rolling history retained for 180 runs.",
    },
    {
      label: "Total requests",
      value: formatNumber(summary.requests_total),
      meta: "Discovery and validation outbound requests.",
    },
    {
      label: "Total proxies checked",
      value: formatNumber(summary.proxies_checked_total),
      meta: "Validation attempts across all retained runs.",
    },
    {
      label: "Total validated",
      value: formatNumber(summary.validated_total),
      meta: "Proxies that passed validation and reached published output.",
    },
    {
      label: "Published HTTP",
      value: formatNumber(current.http),
      meta: "Current root http.txt count.",
    },
    {
      label: "Published SOCKS4",
      value: formatNumber(current.socks4),
      meta: "Current root socks4.txt count.",
    },
    {
      label: "Published SOCKS5",
      value: formatNumber(current.socks5),
      meta: "Current root socks5.txt count.",
    },
    {
      label: "Published total",
      value: formatNumber(current.all),
      meta: "Current root all.txt count.",
    },
  ];

  document.getElementById("kpi-grid").innerHTML = cards
    .map(
      (card) => `
        <article class="kpi-card">
          <p class="kpi-label">${escapeHtml(card.label)}</p>
          <p class="kpi-value">${escapeHtml(card.value)}</p>
          <p class="kpi-meta">${escapeHtml(card.meta)}</p>
        </article>
      `
    )
    .join("");
}

function renderRangeControls(totalRuns) {
  const options = [7, 30, 90, 180];
  const controls = document.getElementById("range-controls");
  controls.innerHTML = options
    .map((value) => {
      const active = state.range === value ? " is-active" : "";
      const label = totalRuns < value ? `${value} max` : `${value} runs`;
      return `<button class="range-button${active}" type="button" data-range="${value}" aria-pressed="${state.range === value}">${label}</button>`;
    })
    .join("");

  controls.querySelectorAll("[data-range]").forEach((button) => {
    button.addEventListener("click", () => {
      state.range = Number(button.dataset.range);
      render();
    });
  });
}

function renderValidatedChart(history) {
  renderChartMetrics("chart-validated-meta", [
    metricChip("Latest", formatNumber(history.at(-1)?.validated || 0)),
    metricChip("Peak", formatNumber(Math.max(...history.map((entry) => entry.validated), 0))),
    metricChip("Avg", formatNumber(Math.round(average(history.map((entry) => entry.validated))))),
  ]);

  renderLineChart(document.getElementById("chart-validated"), {
    history,
    series: [
      {
        key: "validated",
        label: "Validated",
        color: chartColors.validated,
        formatter: (entry) => `${formatNumber(entry.validated)} validated`,
        fillArea: true,
      },
    ],
  });
}

function renderCheckedChart(history) {
  renderChartMetrics("chart-checked-meta", [
    metricChip("Checked", formatNumber(history.at(-1)?.proxies_checked || 0)),
    metricChip("Validated", formatNumber(history.at(-1)?.validated || 0)),
    metricChip("Yield", rate(history.at(-1)?.validated || 0, history.at(-1)?.proxies_checked || 0)),
  ]);

  renderLineChart(document.getElementById("chart-checked"), {
    history,
    series: [
      {
        key: "proxies_checked",
        label: "Checked",
        color: chartColors.checked,
        formatter: (entry) => `${formatNumber(entry.proxies_checked)} checked`,
      },
      {
        key: "validated",
        label: "Validated",
        color: chartColors.validated,
        formatter: (entry) => `${formatNumber(entry.validated)} validated`,
      },
    ],
  });
}

function renderProtocolChart(history) {
  const latest = history.at(-1) || {};
  renderChartMetrics("chart-protocols-meta", [
    metricChip("HTTP", formatNumber(valueAt(latest, "output_counts.http"))),
    metricChip("SOCKS4", formatNumber(valueAt(latest, "output_counts.socks4"))),
    metricChip("SOCKS5", formatNumber(valueAt(latest, "output_counts.socks5"))),
  ]);

  renderLineChart(document.getElementById("chart-protocols"), {
    history,
    series: [
      {
        key: "output_counts.http",
        label: "HTTP",
        color: chartColors.http,
        formatter: (entry) => `${formatNumber(valueAt(entry, "output_counts.http"))} HTTP`,
      },
      {
        key: "output_counts.socks4",
        label: "SOCKS4",
        color: chartColors.socks4,
        formatter: (entry) => `${formatNumber(valueAt(entry, "output_counts.socks4"))} SOCKS4`,
      },
      {
        key: "output_counts.socks5",
        label: "SOCKS5",
        color: chartColors.socks5,
        formatter: (entry) => `${formatNumber(valueAt(entry, "output_counts.socks5"))} SOCKS5`,
      },
    ],
  });
}

function renderChartMetrics(targetId, items) {
  document.getElementById(targetId).innerHTML = items
    .map((item) => `<span class="chart-metric">${escapeHtml(item)}</span>`)
    .join("");
}

function renderRecentRuns(history) {
  const rows = [...history].reverse().slice(0, 12);
  document.getElementById("recent-runs").innerHTML = rows
    .map(
      (entry) => `
        <tr>
          <td>${escapeHtml(formatTimestamp(entry.finished_at))}</td>
          <td><span class="table-status status-${entry.status}">${escapeHtml(humanizeStatus(entry.status))}</span></td>
          <td>${escapeHtml(formatNumber(entry.validated))}</td>
          <td>${escapeHtml(formatNumber(entry.proxies_checked))}</td>
          <td>${escapeHtml(formatNumber(entry.requests_made))}</td>
        </tr>
      `
    )
    .join("");
}

function renderLineChart(target, config) {
  const { history, series } = config;
  if (!history.length) {
    target.innerHTML = `<div class="empty-state">No history available yet.</div>`;
    return;
  }

  const width = 760;
  const height = 274;
  const margin = { top: 24, right: 18, bottom: 36, left: 48 };
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
          <stop offset="0%" stop-color="${item.color}" stop-opacity="0.35"></stop>
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
          return `<circle class="data-point" cx="${point.x.toFixed(2)}" cy="${point.y.toFixed(2)}" r="4.5" fill="${item.color}" data-tooltip="${escapeAttribute(detail)}"><title>${escapeHtml(detail)}</title></circle>`;
        })
        .join("");

      return `
        <g>
          ${item.fillArea ? `<path d="${areaPath}" fill="url(#fill-${target.id}-${seriesIndex})"></path>` : ""}
          <path d="${path}" fill="none" stroke="${item.color}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"></path>
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
      const bandWidth = history.length === 1 ? innerWidth : Math.max(xStep, 18);
      const left = Math.max(margin.left, x - bandWidth / 2);
      const detail = `${formatTimestamp(entry.finished_at)}\nValidated ${formatNumber(entry.validated)}\nChecked ${formatNumber(entry.proxies_checked)}`;
      return `<rect class="hover-band data-point" x="${left.toFixed(2)}" y="${margin.top}" width="${bandWidth.toFixed(2)}" height="${innerHeight}" data-tooltip="${escapeAttribute(detail)}" fill="transparent"></rect>`;
    })
    .join("");

  const xLabels = labelPositions(history, margin.left, xStep)
    .map((label) => `<text class="axis-label" x="${label.x.toFixed(2)}" y="${height - 8}">${escapeHtml(label.text)}</text>`)
    .join("");

  const legend = series
    .map((item, index) => {
      const x = margin.left + index * 124;
      return `
        <g transform="translate(${x}, 10)">
          <circle cx="0" cy="0" r="5" fill="${item.color}"></circle>
          <text class="axis-label" x="12" y="4">${escapeHtml(item.label)}</text>
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
    return [{ x: startX, text: shortDate(history[0].finished_at) }];
  }

  const indexes = [0, Math.floor((history.length - 1) / 2), history.length - 1];
  return indexes.map((index) => ({
    x: startX + index * xStep,
    text: shortDate(history[index].finished_at),
  }));
}

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

function renderError(error) {
  document.getElementById("status-badge").textContent = "Unavailable";
  document.getElementById("status-badge").className = "status-badge status-error";
  document.getElementById("last-updated").textContent = `Failed to load ${DATA_URL}: ${error.message}`;
  document.getElementById("hero-highlights").innerHTML = `<div class="empty-state">Dashboard data could not be loaded.</div>`;
  document.getElementById("run-health").innerHTML = `<div class="empty-state">No run data</div>`;
  document.getElementById("source-mix").innerHTML = `<div class="empty-state">No source data</div>`;
  document.getElementById("kpi-grid").innerHTML = `<div class="empty-state">Dashboard data could not be loaded.</div>`;
  document.getElementById("chart-validated").innerHTML = `<div class="empty-state">No data</div>`;
  document.getElementById("chart-checked").innerHTML = `<div class="empty-state">No data</div>`;
  document.getElementById("chart-protocols").innerHTML = `<div class="empty-state">No data</div>`;
  document.getElementById("recent-runs").innerHTML = `<tr><td colspan="5">No run data available.</td></tr>`;
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

function deltaText(current, previous, suffix) {
  const delta = Number(current || 0) - Number(previous || 0);
  if (delta === 0) {
    return `Flat ${suffix}.`;
  }
  const direction = delta > 0 ? "Up" : "Down";
  return `${direction} ${formatNumber(Math.abs(delta))} ${suffix}.`;
}

function metricChip(label, value) {
  return `${label} ${value}`;
}

function isHealthyStatus(status) {
  return status === "success" || status === "success_with_errors";
}

function humanizeStatus(status) {
  return String(status || "unknown").replaceAll("_", " ");
}

function shortDate(value) {
  const date = new Date(value);
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
  }).format(date);
}

function formatTimestamp(value) {
  if (!value) {
    return "n/a";
  }
  const date = new Date(value);
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
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
