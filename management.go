package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"grok-autoban/cpasdk/pluginapi"
)

type managementHandler struct{}

const managementRoutePrefix = "/plugins/" + pluginID

func managementRegistration() pluginapi.ManagementRegistrationResponse {
	return pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{Method: http.MethodGet, Path: managementRoutePrefix + "/bans", Description: "查看 Grok 401/403 自动禁用账号"},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/unban", Description: "解除单个 Grok 账号禁用"},
			{Method: http.MethodPost, Path: managementRoutePrefix + "/unban-all", Description: "解除全部 Grok 账号禁用"},
		},
		Resources: []pluginapi.ResourceRoute{
			{Path: "/status", Menu: "Grok 自动禁用", Description: "查看 Grok 401/403 自动禁用状态"},
		},
	}
}

func (managementHandler) HandleManagement(_ context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	return dispatchManagement(req)
}

func dispatchManagement(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	switch {
	case req.Method == http.MethodGet && strings.HasSuffix(req.Path, "/bans"):
		return jsonManagementResponse(http.StatusOK, banStatus()), nil
	case req.Method == http.MethodPost && strings.HasSuffix(req.Path, "/unban"):
		var body struct {
			AuthID string `json:"auth_id"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil || strings.TrimSpace(body.AuthID) == "" {
			return jsonManagementResponse(http.StatusBadRequest, map[string]string{"error": "missing_auth_id"}), nil
		}
		authID := strings.TrimSpace(body.AuthID)
		password := resolveManagementPassword(req.Headers)
		enabled, errEnable := enableAuthInCPAAllowMissing(authID, password)
		if errEnable != nil {
			return jsonManagementResponse(http.StatusBadRequest, map[string]string{"error": errEnable.Error()}), nil
		}
		removed := activeStore.Delete(authID)
		saveActiveStore()
		return jsonManagementResponse(http.StatusOK, map[string]any{
			"ok":      true,
			"removed": removed,
			"enabled": enabled,
			"missing": !enabled,
		}), nil
	case req.Method == http.MethodPost && strings.HasSuffix(req.Path, "/unban-all"):
		password := resolveManagementPassword(req.Headers)
		// time.Time{} lists all entries with ResetAt after zero (all future resets).
		items := activeStore.List(time.Time{})
		failures := make([]string, 0)
		enabled := 0
		missing := 0
		for _, entry := range items {
			wasEnabled, errEnable := enableAuthInCPAAllowMissing(entry.AuthID, password)
			if errEnable != nil {
				failures = append(failures, entry.AuthID+": "+errEnable.Error())
				continue
			}
			if wasEnabled {
				enabled++
			} else {
				missing++
			}
			_ = activeStore.Delete(entry.AuthID)
		}
		if len(failures) == 0 {
			activeStore.Clear()
		}
		saveActiveStore()
		status := http.StatusOK
		if len(failures) > 0 && enabled == 0 && missing == 0 {
			status = http.StatusBadRequest
		}
		return jsonManagementResponse(status, map[string]any{
			"ok":       len(failures) == 0,
			"enabled":  enabled,
			"missing":  missing,
			"failed":   len(failures),
			"failures": failures,
		}), nil
	case req.Method == http.MethodGet && strings.HasSuffix(req.Path, "/status"):
		return managementStatusPage(req)
	default:
		return jsonManagementResponse(http.StatusNotFound, map[string]string{"error": "not_found"}), nil
	}
}

func saveActiveStore() {
	cfg := loadedConfig()
	if cfg.PersistState && cfg.StateFile != "" {
		_ = activeStore.Save(cfg.StateFile)
	}
}

func banStatus() map[string]any {
	now := time.Now()
	items := activeStore.List(now)
	out := make([]map[string]any, 0, len(items))
	for _, entry := range items {
		out = append(out, map[string]any{
			"auth_id":           entry.AuthID,
			"provider":          entry.Provider,
			"error_code":        entry.ErrorCode,
			"banned_at":         entry.BannedAt.Format(time.RFC3339),
			"reset_at":          entry.ResetAt.Format(time.RFC3339),
			"reset_source":      entry.ResetSource,
			"trace_id":          entry.TraceID,
			"remaining_seconds": int64(entry.ResetAt.Sub(now).Seconds()),
		})
	}
	return map[string]any{
		"plugin":         pluginID,
		"fallback_hours": loadedConfig().FallbackHours,
		"bans":           out,
	}
}

func jsonManagementResponse(status int, value any) pluginapi.ManagementResponse {
	raw, _ := json.Marshal(value)
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       raw,
	}
}

func managementStatusPage(_ pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	body := `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>Grok 自动禁用</title>
<style>
body{font-family:system-ui,sans-serif;max-width:1080px;margin:32px auto;padding:0 20px;color:#1f2937}
button,select,input{padding:8px 12px;font:inherit}
button{cursor:pointer}
.toolbar{display:flex;flex-wrap:wrap;gap:10px;align-items:center;margin:14px 0}
table{width:100%;border-collapse:collapse;margin-top:12px}
td,th{padding:8px;border-bottom:1px solid #ddd;text-align:left;vertical-align:top}
.muted{color:#64748b;font-size:13px}
.pager{display:flex;flex-wrap:wrap;gap:10px;align-items:center;margin-top:14px}
code{word-break:break-all}
</style></head>
<body>
<h1>Grok 自动禁用</h1>
<p>只处理权限拒绝（403 <code>permission-denied</code>）和认证失败（401）。免费额度耗尽（429）请交给 CPA Manager Plus 的 Provider 额度冷却。</p>
<p class="muted">401/403 需手动解禁，不会自动恢复。</p>
<div class="toolbar">
  <input id="key" type="password" placeholder="CPA Management Key" style="min-width:260px">
  <button onclick="saveKey()">保存密钥</button>
  <button onclick="loadBans()">刷新状态</button>
  <button onclick="unbanAll()">全部解禁</button>
  <label>每页
    <select id="pageSize">
      <option value="20" selected>20</option>
      <option value="50">50</option>
      <option value="100">100</option>
    </select>
  </label>
</div>
<p id="summary" class="muted"></p>
<table>
  <thead>
    <tr><th>账号</th><th>错误码</th><th>禁用时间</th><th>恢复方式</th><th>操作</th></tr>
  </thead>
  <tbody id="rows"></tbody>
</table>
<div class="pager">
  <button id="prevPage" onclick="changePage(-1)">上一页</button>
  <span id="pageInfo" class="muted"></span>
  <button id="nextPage" onclick="changePage(1)">下一页</button>
</div>
<script>
const PREFS_KEY = "grok429ManagementPrefs";
const keyInput = document.getElementById("key");
const pageSizeSelect = document.getElementById("pageSize");
let allBans = [];
let page = 1;

function loadPrefs() {
  try { return JSON.parse(localStorage.getItem(PREFS_KEY) || "{}") || {}; } catch (_) { return {}; }
}
function savePrefs(patch) {
  const next = Object.assign(loadPrefs(), patch || {});
  localStorage.setItem(PREFS_KEY, JSON.stringify(next));
}
const prefs = loadPrefs();
keyInput.value = prefs.managementKey || localStorage.getItem("grok429ManagementKey") || "";
if ([20,50,100].includes(Number(prefs.pageSize))) pageSizeSelect.value = String(prefs.pageSize);
pageSizeSelect.addEventListener("change", () => {
  savePrefs({ pageSize: Number(pageSizeSelect.value) || 20 });
  page = 1;
  renderPage();
});
function saveKey() {
  savePrefs({ managementKey: keyInput.value, pageSize: Number(pageSizeSelect.value) || 20 });
  localStorage.setItem("grok429ManagementKey", keyInput.value);
}
async function call(path, options={}) {
  options.headers = Object.assign({}, options.headers||{}, {
    "Authorization":"Bearer "+keyInput.value,
    "Content-Type":"application/json"
  });
  const r = await fetch("/v0/management/plugins/grok-autoban"+path, options);
  const data = await r.json();
  if (!r.ok) throw new Error((data && (data.error||data.message)) || ("HTTP "+r.status));
  return data;
}
function pageSize() {
  const n = Number(pageSizeSelect.value) || 20;
  return [20,50,100].includes(n) ? n : 20;
}
function totalPages() {
  return Math.max(1, Math.ceil(allBans.length / pageSize()));
}
function renderPage() {
  const size = pageSize();
  const pages = totalPages();
  if (page > pages) page = pages;
  if (page < 1) page = 1;
  const start = (page - 1) * size;
  const slice = allBans.slice(start, start + size);
  document.getElementById("rows").innerHTML = slice.map(b =>
    "<tr>" +
    "<td><code>"+esc(b.auth_id)+"</code></td>" +
    "<td>"+esc(b.error_code || "")+"</td>" +
    "<td>"+esc(b.banned_at || "")+"</td>" +
    "<td>"+esc(b.reset_source || "")+"</td>" +
    "<td><button onclick='unban("+JSON.stringify(b.auth_id)+")'>解禁</button></td>" +
    "</tr>"
  ).join("");
  document.getElementById("summary").textContent = "共 " + allBans.length + " 个禁用账号";
  document.getElementById("pageInfo").textContent = "第 " + page + " / " + pages + " 页";
  document.getElementById("prevPage").disabled = page <= 1;
  document.getElementById("nextPage").disabled = page >= pages;
}
function changePage(delta) {
  page += delta;
  renderPage();
}
async function loadBans() {
  const data = await call("/bans");
  allBans = Array.isArray(data.bans) ? data.bans : [];
  page = 1;
  renderPage();
}
async function unban(id) {
  try {
    const data = await call("/unban",{method:"POST",body:JSON.stringify({auth_id:id})});
    if (data && data.missing) alert("账号已不在 CPA 认证列表中，已清除本插件禁用记录");
  } catch (e) { alert(String(e.message||e)); }
  loadBans();
}
async function unbanAll() {
  try {
    const data = await call("/unban-all",{method:"POST",body:"{}"});
    if (data && data.failed) alert("部分解禁失败: "+(data.failures||[]).slice(0,3).join("; "));
    else if (data && data.missing) alert("已清除 "+data.missing+" 条已删除账号的禁用记录");
  } catch (e) { alert(String(e.message||e)); }
  loadBans();
}
function esc(v) {
  return String(v).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
}
loadBans();
</script>
</body></html>`
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       []byte(body),
	}, nil
}

func handleManagement(raw []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	response, err := dispatchManagement(req)
	if err != nil {
		return nil, err
	}
	return okEnvelope(response)
}
