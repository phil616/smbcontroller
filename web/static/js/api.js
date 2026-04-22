const api = {
  async request(path, options = {}) {
    const init = { credentials: 'same-origin', headers: { ...(options.headers || {}) }, ...options };
    if (init.body && !(init.body instanceof FormData)) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(init.body);
    }
    const response = await fetch(`/api/v1${path}`, init);
    const type = response.headers.get('Content-Type') || '';
    const data = type.includes('application/json') ? await response.json() : await response.text();
    if (!response.ok) {
      const message = humanizeError(data && data.error ? data.error : response.statusText);
      throw new Error(message);
    }
    return data;
  },
  get(path) { return this.request(path); },
  post(path, body) { return this.request(path, { method: 'POST', body }); },
  put(path, body) { return this.request(path, { method: 'PUT', body }); },
  delete(path) { return this.request(path, { method: 'DELETE' }); }
};

function humanizeError(message) {
  const text = String(message || '');
  const lower = text.toLowerCase();
  if (lower.includes('path must be normalized') || lower.includes('path must be normalize')) {
    return '路径必须是规范路径。建议：删除末尾多余斜杠、重复斜杠或 .. 片段。例如将 /srv/samba/ 改为 /srv/samba。解决方案：按建议修改路径后重新提交。';
  }
  if (lower.includes('path must be absolute')) {
    return '路径必须是绝对路径。建议：以 / 开头，例如 /srv/samba/data。解决方案：不要使用 data 或 ./data 这类相对路径。';
  }
  if (lower.includes('username must')) {
    return '用户名格式不正确。建议：使用 3-32 位英文字母、数字或下划线，例如 smb_user01。解决方案：删除空格、中文、连字符或其他特殊字符后重试。';
  }
  if (lower.includes('password must')) {
    return '密码强度不足。建议：至少 8 位，并同时包含大写字母、小写字母和数字。解决方案：补充缺少的字符类型后重试。';
  }
  if (lower.includes('share_name must')) {
    return '共享名格式不正确。建议：使用 1-64 位英文字母、数字、下划线或连字符，例如 DataFolder。解决方案：删除空格、中文或其他特殊字符后重试。';
  }
  return text || '请求失败，请检查输入后重试。';
}
