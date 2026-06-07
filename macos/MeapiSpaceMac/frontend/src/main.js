import {Events} from "@wailsio/runtime";
import {QuotaService} from "../bindings/meapispace-mac";

const app = document.getElementById("app");
const quotaCard = document.getElementById("quota-card");
const collapsedCard = document.getElementById("collapsed-card");
const settingsCard = document.getElementById("settings-card");
const contextMenu = document.getElementById("context-menu");
const toast = document.getElementById("toast");
const startupTip = document.getElementById("startup-tip");
const apiInput = document.getElementById("api-key");
const settingsMessage = document.getElementById("settings-message");

let state = null;
let collapsed = false;
let settingsOpen = false;
let apiDraft = "";
let toastTimer = 0;

Events.On("quota:update", event => {
  applyState(event.data);
});

document.getElementById("refresh").addEventListener("click", async event => {
  event.stopPropagation();
  showToast("刷新中");
  applyState(await QuotaService.Refresh());
});

quotaCard.addEventListener("dblclick", event => {
  if (event.target.closest("button")) return;
  toggleCollapsed();
});

collapsedCard.addEventListener("dblclick", () => toggleCollapsed());

document.addEventListener("contextmenu", event => {
  event.preventDefault();
  showContextMenu(event.clientX, event.clientY);
});

document.addEventListener("click", event => {
  if (!event.target.closest("#context-menu")) {
    contextMenu.hidden = true;
  }
});

contextMenu.addEventListener("click", async event => {
  const action = event.target?.dataset?.action;
  if (!action) return;
  contextMenu.hidden = true;
  if (action === "settings") openSettings();
  if (action === "hide") await QuotaService.HideWindow();
  if (action === "quit") await QuotaService.QuitApp();
});

document.getElementById("settings-close").addEventListener("click", closeSettings);
document.getElementById("cancel").addEventListener("click", closeSettings);
document.getElementById("get-key").addEventListener("click", () => QuotaService.OpenAPIKeyPage());
document.getElementById("save").addEventListener("click", async () => {
  settingsMessage.textContent = "正在保存";
  const next = await QuotaService.SaveAPIKey(apiDraft);
  applyState(next);
  if (next.message !== "保存失败") {
    closeSettings();
    showToast(next.message || "已保存");
  }
});

apiInput.addEventListener("focus", () => {
  apiInput.value = apiDraft;
  apiInput.select();
});

apiInput.addEventListener("input", () => {
  apiDraft = apiInput.value.trim();
});

apiInput.addEventListener("blur", () => {
  apiInput.value = maskKey(apiDraft);
});

window.meapiOpenSettings = openSettings;

setTimeout(() => {
  startupTip.classList.add("hide");
}, 10000);

init();

async function init() {
  applyState(await QuotaService.Initial());
  if (!apiDraft) {
    openSettings();
  } else {
    applyState(await QuotaService.Refresh());
  }
}

function applyState(next) {
  if (!next) return;
  state = next;
  apiDraft = next.apiKey || apiDraft || "";
  document.getElementById("updated").textContent = next.updatedText || "等待刷新";
  document.getElementById("percent").textContent = next.percent || "--";
  document.getElementById("amount").textContent = next.amountText || "--";
  document.getElementById("today-cost").textContent = next.todayCostText || "--";
  document.getElementById("today-token").textContent = next.todayTokenText || "--";
  app.dataset.status = next.status || "yellow";
  document.getElementById("refresh").classList.toggle("spinning", Boolean(next.fetching));

  const progress = Math.max(0, Math.min(1, next.progress || 0));
  document.getElementById("ring").style.setProperty("--progress", `${progress * 360}deg`);
  document.querySelectorAll(".stack-lamp").forEach(lamp => {
    lamp.classList.toggle("active", lamp.dataset.lamp === next.status);
  });
  if (!apiInput.matches(":focus")) {
    apiInput.value = maskKey(apiDraft);
  }
  if (next.message && next.message !== "刷新中") {
    showToast(next.message);
  }
}

async function toggleCollapsed() {
  if (settingsOpen) return;
  collapsed = !collapsed;
  app.classList.toggle("collapsed", collapsed);
  app.classList.toggle("expanded", !collapsed);
  quotaCard.hidden = collapsed;
  collapsedCard.hidden = !collapsed;
  await QuotaService.SetCollapsed(collapsed);
}

async function openSettings() {
  settingsOpen = true;
  collapsed = false;
  app.classList.remove("collapsed", "expanded");
  app.classList.add("settings");
  quotaCard.hidden = true;
  collapsedCard.hidden = true;
  settingsCard.hidden = false;
  settingsMessage.textContent = "默认服务：https://meapi.space";
  await QuotaService.SetSettingsOpen(true);
  apiInput.value = maskKey(apiDraft);
}

async function closeSettings() {
  settingsOpen = false;
  app.classList.remove("settings", "collapsed");
  app.classList.add("expanded");
  settingsCard.hidden = true;
  collapsedCard.hidden = true;
  quotaCard.hidden = false;
  await QuotaService.SetSettingsOpen(false);
}

function showContextMenu(x, y) {
  contextMenu.hidden = false;
  const maxX = window.innerWidth - contextMenu.offsetWidth - 4;
  const maxY = window.innerHeight - contextMenu.offsetHeight - 4;
  contextMenu.style.left = `${Math.max(4, Math.min(x, maxX))}px`;
  contextMenu.style.top = `${Math.max(4, Math.min(y, maxY))}px`;
}

function showToast(message) {
  if (!message) return;
  window.clearTimeout(toastTimer);
  toast.textContent = message;
  toast.hidden = false;
  toastTimer = window.setTimeout(() => {
    toast.hidden = true;
  }, 1600);
}

function maskKey(value) {
  value = (value || "").trim();
  if (!value) return "";
  if (value.length <= 16) return value;
  return `${value.slice(0, 7)}******${value.slice(-7)}`;
}
