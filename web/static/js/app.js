let volumes = [];
let users = [];
let permissions = [];

const $ = (selector) => document.querySelector(selector);

function toast(message) {
  const el = $('#toast');
  el.textContent = message;
  el.classList.add('show');
  setTimeout(() => el.classList.remove('show'), 2600);
}

async function boot() {
  const setup = await api.get('/setup/status');
  if (!setup.initialized) {
    location.href = '/setup';
    return;
  }
  try {
    const me = await api.get('/auth/me');
    $('#session-user').textContent = me.username;
  } catch {
    location.href = '/login';
    return;
  }
  bindUI();
  await refreshAll();
}

function bindUI() {
  document.querySelectorAll('.nav').forEach(btn => btn.addEventListener('click', () => {
    document.querySelectorAll('.nav,.view').forEach(el => el.classList.remove('active'));
    btn.classList.add('active');
    $(`#${btn.dataset.view}`).classList.add('active');
    if (btn.dataset.view === 'system') loadSystem();
  }));
  document.querySelectorAll('[data-modal]').forEach(btn => btn.addEventListener('click', () => $(`#${btn.dataset.modal}`).showModal()));
  document.querySelectorAll('[data-close-dialog]').forEach(btn => btn.addEventListener('click', () => closeDialog(btn.closest('dialog'))));
  $('#logout').addEventListener('click', async () => { await api.post('/auth/logout', {}); location.href = '/login'; });
  $('#save-volume').addEventListener('click', saveVolume);
  $('#save-user').addEventListener('click', saveUser);
  $('#save-temp-user').addEventListener('click', saveTemporaryUser);
  $('#save-password').addEventListener('click', savePassword);
  $('#copy-temp-credential').addEventListener('click', copyTemporaryCredential);
  $('#reload-permissions').addEventListener('click', refreshAll);
  $('#apply-bulk-permissions').addEventListener('click', applyBulkPermissions);
  $('#reload-smbd').addEventListener('click', async () => { await runAction(() => api.post('/system/reload', {}), '已执行 reload'); loadSystem(); });
  $('#restart-smbd').addEventListener('click', async () => { await runAction(() => api.post('/system/restart', {}), '已执行 restart'); loadSystem(); });
  $('#settings-form').addEventListener('submit', saveSettings);
  attachLiveValidation(document);
}

function closeDialog(dialog) {
  if (!dialog) return;
  const form = dialog.querySelector('form');
  if (form) {
    form.reset();
    setFormError(form, '');
    form.querySelectorAll('.field-message').forEach(el => {
      el.textContent = '';
      el.classList.remove('error', 'ok');
    });
    form.querySelectorAll('[data-validate]').forEach(input => input.setCustomValidity(''));
  }
  dialog.close();
}

async function refreshAll() {
  [volumes, users, permissions] = await Promise.all([
    api.get('/volumes'),
    api.get('/users'),
    api.get('/permissions')
  ]);
  renderVolumes();
  renderUsers();
  renderTemporaryUserOptions();
  renderPermissionTools();
  renderPermissions();
}

function renderVolumes() {
  $('#volume-table').innerHTML = volumes.map(v => `
    <tr>
      <td>${escapeHTML(v.share_name)}</td>
      <td>${escapeHTML(v.path)}</td>
      <td><span class="badge ${v.enabled ? 'ok' : ''}">${v.enabled ? '启用' : '禁用'}</span></td>
      <td>${v.guest_ok ? '允许' : '关闭'}</td>
      <td><button data-del-volume="${v.id}">删除</button></td>
    </tr>`).join('');
  document.querySelectorAll('[data-del-volume]').forEach(btn => btn.addEventListener('click', async () => {
    if (!confirm('确认删除该卷配置？不会删除实际目录。')) return;
    await runAction(() => api.delete(`/volumes/${btn.dataset.delVolume}`), '卷已删除');
    refreshAll();
  }));
}

function renderUsers() {
  $('#user-table').innerHTML = users.map(u => `
    <tr>
      <td>${escapeHTML(u.username)}</td>
      <td>${u.is_temporary ? '<span class="badge warn">临时</span>' : '<span class="badge ok">普通</span>'}</td>
      <td>${escapeHTML(u.display_name || '')}</td>
      <td><label class="switch"><input type="checkbox" data-user-enabled="${u.id}" ${u.enabled ? 'checked' : ''}><span></span></label></td>
      <td>${u.expires_at ? formatDateTime(u.expires_at) : '-'}</td>
      <td><button data-password="${u.id}">改密</button><button data-del-user="${u.id}">删除</button></td>
    </tr>`).join('');
  document.querySelectorAll('[data-user-enabled]').forEach(input => input.addEventListener('change', async () => {
    await runAction(() => api.put(`/users/${input.dataset.userEnabled}/enabled`, { enabled: input.checked }), '用户状态已更新');
    refreshAll();
  }));
  document.querySelectorAll('[data-password]').forEach(btn => btn.addEventListener('click', () => {
    $('#password-form input[name="id"]').value = btn.dataset.password;
    $('#password-modal').showModal();
  }));
  document.querySelectorAll('[data-del-user]').forEach(btn => btn.addEventListener('click', async () => {
    if (!confirm('确认删除该 SMB 用户？')) return;
    await runAction(() => api.delete(`/users/${btn.dataset.delUser}`), '用户已删除');
    refreshAll();
  }));
}

function renderTemporaryUserOptions() {
  const options = ['<option value="">不自动授权</option>', ...volumes.map(v => `<option value="${v.id}">${escapeHTML(v.share_name)}${v.enabled ? '' : '（禁用）'}</option>`)];
  $('#temp-user-volume').innerHTML = options.join('');
}

function renderPermissions() {
  const byKey = new Map(permissions.map(p => [`${p.user_id}:${p.volume_id}`, p.access]));
  renderPermissionSummary(byKey);
  renderPermissionDetail(byKey);
  if (!users.length || !volumes.length) {
    $('#permission-matrix').innerHTML = '<div class="empty">需要至少一个用户和一个卷</div>';
    return;
  }
  const head = `<div class="cell head"></div>${volumes.map(v => `<div class="cell head">${escapeHTML(v.share_name)}</div>`).join('')}`;
  const rows = users.map(u => `<div class="cell user">${escapeHTML(u.username)}</div>` + volumes.map(v => {
    const access = byKey.get(`${u.id}:${v.id}`) || 'none';
    const disabled = !u.enabled || !v.enabled;
    const title = disabled ? '用户或卷已禁用，修改会保存到数据库，但不会写入当前 Samba 有效配置。' : '选择后立即保存权限。';
    return `<select class="cell select" data-user="${u.id}" data-volume="${v.id}" data-prev="${access}" title="${title}">
      <option value="none" ${access === 'none' ? 'selected' : ''}>无权限</option>
      <option value="read" ${access === 'read' ? 'selected' : ''}>只读</option>
      <option value="readwrite" ${access === 'readwrite' ? 'selected' : ''}>读写</option>
    </select>`;
  }).join('')).join('');
  $('#permission-matrix').style.gridTemplateColumns = `160px repeat(${volumes.length}, minmax(130px, 1fr))`;
  $('#permission-matrix').innerHTML = head + rows;
  document.querySelectorAll('#permission-matrix select').forEach(select => select.addEventListener('change', async () => {
    const prev = select.dataset.prev;
    select.disabled = true;
    const ok = await savePermission(Number(select.dataset.user), Number(select.dataset.volume), select.value);
    if (!ok) select.value = prev;
    select.disabled = false;
    await reloadPermissionsOnly();
  }));
}

function renderPermissionTools() {
  const userOptions = ['<option value="all">全部用户</option>', ...users.map(u => `<option value="${u.id}">${escapeHTML(u.username)}${u.enabled ? '' : '（禁用）'}</option>`)];
  const volumeOptions = ['<option value="all">全部卷</option>', ...volumes.map(v => `<option value="${v.id}">${escapeHTML(v.share_name)}${v.enabled ? '' : '（禁用）'}</option>`)];
  $('#bulk-user').innerHTML = userOptions.join('');
  $('#bulk-volume').innerHTML = volumeOptions.join('');
}

function renderPermissionSummary(byKey) {
  const total = users.length * volumes.length;
  let read = 0;
  let readwrite = 0;
  byKey.forEach(access => {
    if (access === 'read') read++;
    if (access === 'readwrite') readwrite++;
  });
  const none = Math.max(0, total - read - readwrite);
  $('#permission-summary').innerHTML = `
    <div><span>用户</span><strong>${users.length}</strong></div>
    <div><span>卷</span><strong>${volumes.length}</strong></div>
    <div><span>只读</span><strong>${read}</strong></div>
    <div><span>读写</span><strong>${readwrite}</strong></div>
    <div><span>无权限</span><strong>${none}</strong></div>`;
}

function renderPermissionDetail(byKey) {
  if (!users.length || !volumes.length) {
    $('#permission-detail').innerHTML = '';
    return;
  }
  const userRows = users.map(u => {
    const grants = volumes
      .map(v => ({ volume: v, access: byKey.get(`${u.id}:${v.id}`) || 'none' }))
      .filter(item => item.access !== 'none')
      .map(item => `${escapeHTML(item.volume.share_name)}：${accessLabel(item.access)}`)
      .join('，') || '无已授权卷';
    return `<li><strong>${escapeHTML(u.username)}</strong><span>${grants}</span></li>`;
  }).join('');
  const volumeRows = volumes.map(v => {
    const grants = users
      .map(u => ({ user: u, access: byKey.get(`${u.id}:${v.id}`) || 'none' }))
      .filter(item => item.access !== 'none')
      .map(item => `${escapeHTML(item.user.username)}：${accessLabel(item.access)}`)
      .join('，') || '无已授权用户';
    return `<li><strong>${escapeHTML(v.share_name)}</strong><span>${grants}</span></li>`;
  }).join('');
  $('#permission-detail').innerHTML = `
    <section><h3>按用户查看</h3><ul>${userRows}</ul></section>
    <section><h3>按卷查看</h3><ul>${volumeRows}</ul></section>`;
}

async function savePermission(userID, volumeID, access) {
  const result = await runAction(() => api.put('/permissions', { user_id: userID, volume_id: volumeID, access }), '权限已更新');
  return result;
}

async function reloadPermissionsOnly() {
  permissions = await api.get('/permissions');
  renderPermissions();
}

async function applyBulkPermissions(event) {
  event.preventDefault();
  if (!users.length || !volumes.length) {
    toast('需要至少一个用户和一个卷');
    return;
  }
  const userValue = $('#bulk-user').value;
  const volumeValue = $('#bulk-volume').value;
  const userIDs = userValue === 'all' ? users.map(u => u.id) : [Number(userValue)];
  const volumeIDs = volumeValue === 'all' ? volumes.map(v => v.id) : [Number(volumeValue)];
  const access = $('#bulk-access').value;
  const ok = await runAction(() => api.put('/permissions/bulk', { user_ids: userIDs, volume_ids: volumeIDs, access }), '批量权限已更新');
  if (ok) await reloadPermissionsOnly();
}

async function saveVolume(event) {
  event.preventDefault();
  if (!validateForm($('#volume-form'))) return;
  const form = new FormData($('#volume-form'));
  const ok = await runAction(() => api.post('/volumes', {
    share_name: form.get('share_name'),
    path: form.get('path'),
    comment: form.get('comment'),
    browseable: form.has('browseable'),
    guest_ok: form.has('guest_ok'),
    create_dir_if_not_exists: form.has('create_dir_if_not_exists')
  }), '卷已创建', $('#volume-form'));
  if (!ok) return;
  $('#volume-modal').close();
  $('#volume-form').reset();
  refreshAll();
}

async function saveUser(event) {
  event.preventDefault();
  if (!validateForm($('#user-form'))) return;
  const form = new FormData($('#user-form'));
  const ok = await runAction(() => api.post('/users', { username: form.get('username'), display_name: form.get('display_name'), password: form.get('password') }), '用户已创建', $('#user-form'));
  if (!ok) return;
  $('#user-modal').close();
  $('#user-form').reset();
  refreshAll();
}

async function saveTemporaryUser(event) {
  event.preventDefault();
  const formEl = $('#temp-user-form');
  const form = new FormData(formEl);
  const durationHours = Number(form.get('duration_hours'));
  if (!Number.isInteger(durationHours) || durationHours < 1 || durationHours > 336) {
    setFormError(formEl, '临时用户有效期必须在 1 小时到 14 天之间。');
    return;
  }
  const volumeID = form.get('volume_id');
  const access = form.get('access');
  const body = {
    duration_hours: durationHours,
    volume_ids: volumeID && access !== 'none' ? [Number(volumeID)] : [],
    access
  };
  const resp = await api.post('/users/temporary', body).catch(error => {
    toast(error.message);
    setFormError(formEl, error.message);
    return null;
  });
  if (!resp) return;
  closeDialog($('#temp-user-modal'));
  showTemporaryCredential(resp);
  await refreshAll();
}

function showTemporaryCredential(resp) {
  $('#temp-credential-username').value = resp.username;
  $('#temp-credential-password').value = resp.password;
  $('#temp-credential-expires').value = formatDateTime(resp.expires_at);
  $('#temp-credential-modal').showModal();
}

async function copyTemporaryCredential() {
  const text = `SMB 临时用户\n用户名: ${$('#temp-credential-username').value}\n密码: ${$('#temp-credential-password').value}\n过期时间: ${$('#temp-credential-expires').value}`;
  try {
    await navigator.clipboard.writeText(text);
    toast('凭据已复制');
  } catch {
    toast('浏览器不允许复制，请手动选中复制');
  }
}

async function savePassword(event) {
  event.preventDefault();
  if (!validateForm($('#password-form'))) return;
  const form = new FormData($('#password-form'));
  const ok = await runAction(() => api.post(`/users/${form.get('id')}/password`, { new_password: form.get('new_password') }), '密码已更新', $('#password-form'));
  if (!ok) return;
  $('#password-modal').close();
  $('#password-form').reset();
}

async function loadSystem() {
  const [status, settings, conf] = await Promise.all([api.get('/system/status'), api.get('/system/settings'), api.get('/system/conf').catch(() => ({ content: '' }))]);
  $('#system-status').innerHTML = `
    <div><span>smbd</span><strong>${status.smbd_installed ? '已安装' : '未安装'}</strong></div>
    <div><span>运行状态</span><strong>${status.smbd_running ? '运行中' : '未运行'}</strong></div>
    <div><span>版本</span><strong>${escapeHTML(status.smbd_version || '-')}</strong></div>
    <div><span>配置</span><strong>${escapeHTML(status.conf_path)}</strong></div>
    <div><span>卷</span><strong>${status.managed_shares_count}</strong></div>
    <div><span>用户</span><strong>${status.managed_users_count}</strong></div>`;
  const form = $('#settings-form');
  form.elements.smb_workgroup.value = settings.smb_workgroup || 'WORKGROUP';
  form.elements.smb_server_string.value = settings.smb_server_string || 'SMB Controller';
  form.elements.smb_netbios_name.value = settings.smb_netbios_name || '';
  $('#conf-preview').textContent = conf.content || '';
}

async function saveSettings(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  await runAction(() => api.put('/system/settings', {
    smb_workgroup: form.get('smb_workgroup'),
    smb_server_string: form.get('smb_server_string'),
    smb_netbios_name: form.get('smb_netbios_name')
  }), '设置已保存');
  loadSystem();
}

async function runAction(fn, message, form = null) {
  setFormError(form, '');
  try {
    const result = await fn();
    if (result && result.config_applied === false && result.config_error) {
      toast(`${message}；但 Samba 配置未应用：${humanizeError(result.config_error)}`);
    } else {
      toast(message);
    }
    return true;
  } catch (error) {
    toast(error.message);
    setFormError(form, error.message);
    return false;
  }
}

function setFormError(form, message) {
  if (!form) return;
  const el = form.querySelector('[data-form-error]');
  if (!el) return;
  el.textContent = message;
  el.classList.toggle('show', Boolean(message));
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function accessLabel(access) {
  if (access === 'read') return '只读';
  if (access === 'readwrite') return '读写';
  return '无权限';
}

function formatDateTime(value) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString('zh-CN', { hour12: false });
}

boot();
