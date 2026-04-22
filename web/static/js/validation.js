const validationRules = {
  username(value) {
    if (!/^[A-Za-z0-9_]{3,32}$/.test(value)) {
      return '用户名格式不正确。建议使用 3-32 位英文字母、数字或下划线，例如 smb_user01。解决方案：删除空格、中文、连字符或其他特殊字符。';
    }
    return '';
  },
  password(value) {
    if (value.length < 8) {
      return '密码强度不足。建议至少 8 位，并包含大写字母、小写字母和数字。';
    }
    if (!/[A-Z]/.test(value) || !/[a-z]/.test(value) || !/[0-9]/.test(value)) {
      return '密码强度不足。解决方案：同时加入大写字母、小写字母和数字，例如 Admin123。';
    }
    return '';
  },
  share(value) {
    if (!/^[A-Za-z0-9_-]{1,64}$/.test(value)) {
      return '共享名格式不正确。建议使用英文字母、数字、下划线或连字符，例如 DataFolder。解决方案：删除空格、中文或其他特殊字符。';
    }
    return '';
  },
  path(value) {
    if (!value.startsWith('/')) {
      return '路径必须是绝对路径。建议以 / 开头，例如 /srv/samba/data。解决方案：不要使用 data 或 ./data 这类相对路径。';
    }
    const normalized = normalizeUnixPath(value);
    if (normalized !== value) {
      return `路径必须是规范路径。建议使用 ${normalized}。解决方案：删除末尾多余斜杠、重复斜杠或 .. 片段。`;
    }
    return '';
  },
  'confirm-password'(value, input) {
    const form = input.closest('form');
    const password = form ? form.querySelector('input[name="password"]') : null;
    if (password && value !== password.value) {
      return '两次输入的密码不一致。解决方案：重新输入确认密码。';
    }
    return '';
  }
};

function attachLiveValidation(root = document) {
  root.querySelectorAll('[data-validate]').forEach(input => {
    input.addEventListener('input', () => validateInput(input));
    input.addEventListener('blur', () => validateInput(input));
  });
}

function validateForm(form) {
  let ok = true;
  form.querySelectorAll('[data-validate]').forEach(input => {
    if (!validateInput(input)) ok = false;
  });
  return ok;
}

function validateInput(input) {
  const rule = validationRules[input.dataset.validate];
  if (!rule) return true;
  const message = rule(input.value.trim(), input);
  input.setCustomValidity(message);
  const messageEl = input.closest('form')?.querySelector(`.field-message[data-for="${input.name}"]`);
  if (messageEl) {
    messageEl.textContent = input.value || input.required ? message : '';
    messageEl.classList.toggle('error', Boolean(message));
    messageEl.classList.toggle('ok', Boolean(input.value && !message));
    if (input.value && !message) messageEl.textContent = '格式正确';
  }
  return !message;
}

function normalizeUnixPath(value) {
  const absolute = value.startsWith('/');
  const parts = [];
  for (const part of value.split('/')) {
    if (!part || part === '.') continue;
    if (part === '..') {
      parts.pop();
      continue;
    }
    parts.push(part);
  }
  const normalized = `${absolute ? '/' : ''}${parts.join('/')}`;
  return normalized || '/';
}
