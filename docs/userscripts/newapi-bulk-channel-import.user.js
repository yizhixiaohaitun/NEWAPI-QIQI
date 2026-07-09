// ==UserScript==
// @name         NewAPI Bulk Channel Importer
// @namespace    https://github.com/QuantumNous/new-api
// @version      0.5.0
// @description  Bulk-create NewAPI channels from provider keys using the built-in /api/channel API, compatible with NewAPI v0.13.2 and v1.x.
// @match        http://*/channels*
// @match        https://*/channels*
// @match        http://*/channel*
// @match        https://*/channel*
// @match        http://*/*channel*
// @match        https://*/*channel*
// @run-at       document-idle
// @grant        none
// ==/UserScript==

(() => {
  'use strict';

  const SCRIPT_ID = 'nai-bulk-channel-importer';
  const SCRIPT_VERSION = '0.5.0';
  const TOOL_MARK = 'NACP';
  const STORAGE_KEY = 'nai:bulk-channel-importer:v1';
  const WORKSPACE_STORAGE_KEY = 'nai:bulk-channel-importer:workspace:v1';
  const BUTTON_POSITION_KEY = 'nai:bulk-channel-importer:button-position:v1';
  const API_ROOT = '/api/channel';
  const GROUPS_API = '/api/group/';
  const TEMPLATE_PAGE_SIZE = 100;
  const NAME_SLOT_LETTERS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
  const MAX_NAME_SEGMENTS = 12;
  const NAME_SEGMENT_TYPES = [
    ['', '空'],
    ['text', '固定文字'],
    ['num', '顺序数字'],
    ['alpha', '顺序字母'],
    ['rand6', '六位随机码'],
    ['ts', '时间戳'],
    ['date', '日期'],
    ['key8', 'key 前 8 位'],
  ];

  const CHANNEL_TYPES = [
    [1, 'OpenAI'],
    [2, 'Midjourney'],
    [3, 'Azure'],
    [4, 'Ollama'],
    [5, 'MidjourneyPlus'],
    [6, 'OpenAIMax'],
    [7, 'OhMyGPT'],
    [8, 'Custom'],
    [9, 'AILS'],
    [10, 'AIProxy'],
    [11, 'PaLM'],
    [12, 'API2GPT'],
    [13, 'AIGC2D'],
    [14, 'Anthropic'],
    [15, 'Baidu'],
    [16, 'Zhipu'],
    [17, 'Ali'],
    [18, 'Xunfei'],
    [19, '360'],
    [20, 'OpenRouter'],
    [21, 'AIProxyLibrary'],
    [22, 'FastGPT'],
    [23, 'Tencent'],
    [24, 'Gemini'],
    [25, 'Moonshot'],
    [26, 'ZhipuV4'],
    [27, 'Perplexity'],
    [31, 'LingYiWanWu'],
    [33, 'AWS'],
    [34, 'Cohere'],
    [35, 'MiniMax'],
    [36, 'SunoAPI'],
    [37, 'Dify'],
    [38, 'Jina'],
    [39, 'Cloudflare'],
    [40, 'SiliconFlow'],
    [41, 'VertexAI'],
    [42, 'Mistral'],
    [43, 'DeepSeek'],
    [44, 'MokaAI'],
    [45, 'VolcEngine'],
    [46, 'BaiduV2'],
    [47, 'Xinference'],
    [48, 'xAI'],
    [49, 'Coze'],
    [50, 'Kling'],
    [51, 'Jimeng'],
    [52, 'Vidu'],
    [53, 'Submodel'],
    [54, 'DoubaoVideo'],
    [55, 'Sora'],
    [56, 'Replicate'],
    [57, 'ChatGPT Subscription (Codex)'],
    [58, 'Advanced Custom'],
  ];

  const CHANNEL_BASE_URLS = {
    1: 'https://api.openai.com',
    2: 'https://oa.api2d.net',
    3: '',
    4: 'http://localhost:11434',
    5: 'https://api.openai-sb.com',
    6: 'https://api.openaimax.com',
    7: 'https://api.ohmygpt.com',
    8: '',
    9: 'https://api.caipacity.com',
    10: 'https://api.aiproxy.io',
    11: '',
    12: 'https://api.api2gpt.com',
    13: 'https://api.aigc2d.com',
    14: 'https://api.anthropic.com',
    15: 'https://aip.baidubce.com',
    16: 'https://open.bigmodel.cn',
    17: 'https://dashscope.aliyuncs.com',
    18: '',
    19: 'https://api.360.cn',
    20: 'https://openrouter.ai/api',
    21: 'https://api.aiproxy.io',
    22: 'https://fastgpt.run/api/openapi',
    23: 'https://hunyuan.tencentcloudapi.com',
    24: 'https://generativelanguage.googleapis.com',
    25: 'https://api.moonshot.cn',
    26: 'https://open.bigmodel.cn',
    27: 'https://api.perplexity.ai',
    31: 'https://api.lingyiwanwu.com',
    33: '',
    34: 'https://api.cohere.ai',
    35: 'https://api.minimax.chat',
    36: '',
    37: 'https://api.dify.ai',
    38: 'https://api.jina.ai',
    39: 'https://api.cloudflare.com',
    40: 'https://api.siliconflow.cn',
    41: '',
    42: 'https://api.mistral.ai',
    43: 'https://api.deepseek.com',
    44: 'https://api.moka.ai',
    45: 'https://ark.cn-beijing.volces.com',
    46: 'https://qianfan.baidubce.com',
    47: '',
    48: 'https://api.x.ai',
    49: 'https://api.coze.cn',
    50: 'https://api.klingai.com',
    51: 'https://visual.volcengineapi.com',
    52: 'https://api.vidu.cn',
    53: 'https://llm.submodel.ai',
    54: 'https://ark.cn-beijing.volces.com',
    55: 'https://api.openai.com',
    56: 'https://api.replicate.com',
    57: 'https://chatgpt.com',
    58: '',
  };

  const CHANNEL_TYPE_ICONS = {
    1: 'OpenAI',
    2: 'Midjourney',
    3: 'Azure',
    4: 'Ollama',
    5: 'Midjourney',
    6: 'OpenAI',
    7: 'OpenAI',
    8: 'OpenAI',
    9: 'OpenAI',
    10: 'OpenAI',
    11: 'Google',
    12: 'OpenAI',
    13: 'OpenAI',
    14: 'Claude',
    15: 'Baidu',
    16: 'Zhipu',
    17: 'Qwen',
    18: 'Spark',
    19: 'Ai360',
    20: 'OpenRouter',
    21: 'OpenAI',
    22: 'FastGPT',
    23: 'Hunyuan',
    24: 'Gemini',
    25: 'Moonshot',
    26: 'Zhipu',
    27: 'Perplexity',
    31: 'Yi',
    33: 'Aws',
    34: 'Cohere',
    35: 'Minimax',
    36: 'Suno',
    37: 'Dify',
    38: 'Jina',
    39: 'Cloudflare',
    40: 'SiliconCloud',
    41: 'Gemini',
    42: 'Mistral',
    43: 'DeepSeek',
    44: 'OpenAI',
    45: 'Volcengine',
    46: 'Baidu',
    47: 'Xinference',
    48: 'XAI',
    49: 'Coze',
    50: 'Kling',
    51: 'Jimeng',
    52: 'Vidu',
    53: 'OpenAI',
    54: 'Doubao',
    55: 'OpenAI',
    56: 'Replicate',
    57: 'OpenAI',
    58: 'NewAPI',
  };

  const CHANNEL_ICON_META = {
    Ai360: ['360', '#e8f6ef', '#1e8d57'],
    Aws: ['AWS', '#fff4dc', '#f59e0b'],
    Azure: ['AZ', '#e7f1ff', '#2563eb'],
    Baidu: ['BD', '#e9efff', '#3158d4'],
    Claude: ['CL', '#fff0e6', '#d97843'],
    Cloudflare: ['CF', '#fff1d6', '#f59e0b'],
    Cohere: ['CO', '#e8f7ef', '#14955f'],
    Coze: ['CZ', '#e9f3ff', '#1682d4'],
    DeepSeek: ['DS', '#e9f2ff', '#3b82f6'],
    Dify: ['DF', '#eef2ff', '#6366f1'],
    Doubao: ['DB', '#fff0ea', '#f97316'],
    FastGPT: ['FG', '#e8f7f3', '#0f9f85'],
    Gemini: ['GM', '#eee9ff', '#7c3aed'],
    Google: ['GO', '#e9f5ff', '#4285f4'],
    Hunyuan: ['HY', '#eaf7ff', '#0891b2'],
    Jimeng: ['JM', '#fff0f4', '#e11d48'],
    Jina: ['JN', '#ecfeff', '#0891b2'],
    Kling: ['KL', '#f4f2ff', '#6d5dfc'],
    Midjourney: ['MJ', '#f2f0ea', '#6b5f4a'],
    Minimax: ['MM', '#fff4e6', '#d97706'],
    Mistral: ['MI', '#fff5d7', '#ca8a04'],
    Moonshot: ['MS', '#eef2ff', '#4f46e5'],
    NewAPI: ['NA', '#fff0e6', '#e87046'],
    Ollama: ['OL', '#e8ecef', '#475569'],
    OpenAI: ['AI', '#e9f8ef', '#10a37f'],
    OpenRouter: ['OR', '#ede9fe', '#7c3aed'],
    Perplexity: ['PX', '#e6fbff', '#0891b2'],
    Qwen: ['QW', '#e8f0ff', '#2f6fed'],
    Replicate: ['RP', '#f1f5f9', '#475569'],
    SiliconCloud: ['SF', '#e9f7ef', '#16a34a'],
    Spark: ['XF', '#fff1f2', '#e11d48'],
    Suno: ['SU', '#fff7ed', '#ea580c'],
    Vidu: ['VD', '#fdf2f8', '#db2777'],
    Volcengine: ['VE', '#fff0ea', '#f97316'],
    XAI: ['XA', '#f1f5f9', '#111827'],
    Xinference: ['XI', '#f0f9ff', '#0284c7'],
    Yi: ['YI', '#f0fdf4', '#16a34a'],
    Zhipu: ['ZP', '#eef2ff', '#4f46e5'],
  };

  const DEFAULT_SETTING_JSON = JSON.stringify({
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    system_prompt: '',
    system_prompt_override: false,
  });

  const DEFAULT_CONFIG = {
    jobName: '',
    typePreset: '14',
    nameSegments: ['text', 'num'],
    nameSegmentSettings: {
      A: {
        text: 'Anthropic-',
        numberStart: '1',
        numberPad: '2',
        alphaStart: 'A',
        tsFormat: 'yyyyMMdd-HHmmss',
        dateFormat: 'yyyyMMdd',
      },
      B: {
        text: '',
        numberStart: '1',
        numberPad: '2',
        alphaStart: 'A',
        tsFormat: 'yyyyMMdd-HHmmss',
        dateFormat: 'yyyyMMdd',
      },
    },
    models: '',
    group: 'default',
    modelMapping: '',
    priority: '0',
    weight: '0',
    tag: '',
    remark: '',
    status: true,
    autoBan: true,
    delayMs: '250',
    autoRefill: true,
    targetAliveSize: '10',
    replenishBatchSize: '10',
    aliveThreshold: '5',
    monitorIntervalSec: '60',
    exportRawKeys: false,
    dedupeKeys: true,
    continueOnError: false,
    allowServiceTier: false,
    allowInferenceGeo: false,
    allowSpeed: false,
    claudeBetaQuery: false,
    settingJson: DEFAULT_SETTING_JSON,
    settingsJson: '{}',
    paramOverride: '',
    headerOverride: '',
    statusCodeMapping: '',
    other: '',
  };

  const state = {
    open: false,
    running: false,
    nameSeedKey: '',
    nameTimestamp: '',
    nameDate: '',
    randomCodes: new Map(),
    groups: [],
    groupsLoaded: false,
    templates: [],
    templatesLoaded: false,
    keyPool: [],
    keyPoolSet: new Set(),
    activeJob: null,
    workLogs: [],
    strategyDirty: false,
    monitorTimer: null,
    monitorBusy: false,
    hostTypeIcons: {},
    buttonDrag: null,
    buttonClickSuppressedUntil: 0,
  };

  const fieldIds = [
    'jobName',
    'typePreset',
    'models',
    'group',
    'modelMapping',
    'priority',
    'weight',
    'tag',
    'remark',
    'delayMs',
    'targetAliveSize',
    'replenishBatchSize',
    'aliveThreshold',
    'monitorIntervalSec',
    'settingJson',
    'settingsJson',
    'paramOverride',
    'headerOverride',
    'statusCodeMapping',
    'other',
  ];

  const checkboxIds = [
    'status',
    'autoBan',
    'autoRefill',
    'exportRawKeys',
    'dedupeKeys',
    'continueOnError',
    'allowServiceTier',
    'allowInferenceGeo',
    'allowSpeed',
    'claudeBetaQuery',
  ];

  function qs(selector, root = document) {
    return root.querySelector(selector);
  }

  function qsa(selector, root = document) {
    return Array.from(root.querySelectorAll(selector));
  }

  function escapeHtml(value) {
    return String(value ?? '')
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#039;');
  }

  function cloneValue(value) {
    return JSON.parse(JSON.stringify(value));
  }

  function slotLabel(index) {
    return NAME_SLOT_LETTERS[index] || String(index + 1);
  }

  function defaultSegmentSettings(slot) {
    const base = DEFAULT_CONFIG.nameSegmentSettings[slot] || DEFAULT_CONFIG.nameSegmentSettings.A;
    return { ...base };
  }

  function normalizeSegmentType(value) {
    const text = String(value || '').trim();
    if (text === 'prefix' || text === 'suffix' || /^text\d*$/.test(text)) return 'text';
    return NAME_SEGMENT_TYPES.some(([type]) => type === text) ? text : '';
  }

  function oldNameSegmentsFromConfig(config) {
    const parts = [
      config.namePart1,
      config.namePart2,
      config.namePart3,
      config.namePart4,
      config.namePart5,
    ].map(normalizeSegmentType).filter(Boolean);
    return parts.length ? parts : cloneValue(DEFAULT_CONFIG.nameSegments);
  }

  function normalizeNameSegments(value, fallbackConfig = null) {
    let segments = [];
    if (Array.isArray(value)) {
      segments = value.map(normalizeSegmentType);
    } else if (typeof value === 'string' && value.trim()) {
      try {
        const parsed = JSON.parse(value);
        if (Array.isArray(parsed)) segments = parsed.map(normalizeSegmentType);
      } catch {
        segments = value.split(',').map(normalizeSegmentType);
      }
    }
    if (!segments.length && fallbackConfig) segments = oldNameSegmentsFromConfig(fallbackConfig);
    if (!segments.length) segments = cloneValue(DEFAULT_CONFIG.nameSegments);
    return segments.slice(0, MAX_NAME_SEGMENTS);
  }

  function normalizeNameSegmentSettings(settings = {}, legacyConfig = {}) {
    const normalized = {};
    for (let index = 0; index < MAX_NAME_SEGMENTS; index += 1) {
      const slot = slotLabel(index);
      normalized[slot] = {
        ...defaultSegmentSettings(slot),
        ...(settings && typeof settings === 'object' ? settings[slot] || {} : {}),
      };
    }

    if (legacyConfig.nameText1 !== undefined || legacyConfig.prefix !== undefined) {
      normalized.A.text = String(legacyConfig.nameText1 ?? legacyConfig.prefix ?? normalized.A.text ?? '');
    }
    if (legacyConfig.nameText2 !== undefined || legacyConfig.suffix !== undefined) {
      normalized.B.text = String(legacyConfig.nameText2 ?? legacyConfig.suffix ?? normalized.B.text ?? '');
    }
    if (legacyConfig.nameText3 !== undefined) {
      normalized.C.text = String(legacyConfig.nameText3 ?? '');
    }
    if (legacyConfig.numberStart !== undefined) {
      for (const slot of Object.keys(normalized)) normalized[slot].numberStart = String(legacyConfig.numberStart);
    }
    if (legacyConfig.numberPad !== undefined) {
      for (const slot of Object.keys(normalized)) normalized[slot].numberPad = String(legacyConfig.numberPad);
    }
    if (legacyConfig.alphaStart !== undefined) {
      for (const slot of Object.keys(normalized)) normalized[slot].alphaStart = String(legacyConfig.alphaStart);
    }

    return normalized;
  }

  function loadConfig() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return { ...DEFAULT_CONFIG };
      const parsed = JSON.parse(raw);
      const config = { ...DEFAULT_CONFIG, ...parsed };
      if (config.typePreset === 'custom') {
        config.typePreset = CHANNEL_TYPES.some(([value]) => String(value) === String(config.customType))
          ? String(config.customType)
          : DEFAULT_CONFIG.typePreset;
      }
      if (!CHANNEL_TYPES.some(([value]) => String(value) === String(config.typePreset))) {
        config.typePreset = DEFAULT_CONFIG.typePreset;
      }
      config.nameSegments = normalizeNameSegments(parsed.nameSegments, config);
      config.nameSegmentSettings = normalizeNameSegmentSettings(parsed.nameSegmentSettings, config);
      return config;
    } catch {
      return { ...DEFAULT_CONFIG };
    }
  }

  function saveConfig(config) {
    const sanitized = { ...config };
    delete sanitized.keys;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(sanitized));
  }

  function jobForStorage(job) {
    if (!job) return null;
    const { keys, ...rest } = job;
    return rest;
  }

  function normalizeWorkspacePayload(payload) {
    const data = payload && typeof payload === 'object' ? payload : {};
    const keyPool = Array.isArray(data.keyPool) ? data.keyPool : [];
    const activeJob = data.activeJob && typeof data.activeJob === 'object' ? data.activeJob : null;
    const workLogs = Array.isArray(data.workLogs) ? data.workLogs : [];
    return { keyPool, activeJob, workLogs };
  }

  function applyWorkspacePayload(payload, options = {}) {
    const normalized = normalizeWorkspacePayload(payload);
    state.keyPool = normalized.keyPool.map((entry) => ({
      key: String(entry.key || ''),
      keyPreview: entry.keyPreview || keyPreview(entry.key || ''),
      addedAt: entry.addedAt || nowIso(),
      attemptedAt: entry.attemptedAt || null,
      usedAt: entry.usedAt || null,
      channelCreatedAt: entry.channelCreatedAt || null,
      channelId: entry.channelId ?? null,
      channelName: entry.channelName || '',
      status: entry.status ?? null,
      statusText: entry.statusText || '未使用',
      disabledAt: entry.disabledAt || null,
      lastSeenAt: entry.lastSeenAt || null,
      usedQuota: Number(entry.usedQuota || 0),
      batchNo: entry.batchNo ?? null,
      error: entry.error || '',
    })).filter((entry) => entry.key);
    state.keyPoolSet = new Set(state.keyPool.map((entry) => entry.key));
    state.activeJob = normalized.activeJob ? { ...normalized.activeJob, keys: state.keyPool } : null;
    if (state.activeJob && !state.activeJob.name) {
      state.activeJob.name = state.activeJob.configSnapshot?.jobName || state.activeJob.id || defaultJobName(state.activeJob.configSnapshot || DEFAULT_CONFIG);
    }
    if (state.activeJob?.runtimeConfig) {
      state.activeJob.runtimeConfig = {
        autoRefill: state.activeJob.runtimeConfig.autoRefill !== false,
        targetAliveSize: parsePositiveInt(state.activeJob.runtimeConfig.targetAliveSize, 10, 0),
        aliveThreshold: parsePositiveInt(state.activeJob.runtimeConfig.aliveThreshold, 5, 0),
        replenishBatchSize: parsePositiveInt(state.activeJob.runtimeConfig.replenishBatchSize, 10, 1),
        monitorIntervalSec: parsePositiveInt(state.activeJob.runtimeConfig.monitorIntervalSec, 60, 5),
        updatedAt: state.activeJob.runtimeConfig.updatedAt || nowIso(),
      };
    }
    state.workLogs = normalized.workLogs.map((entry) => ({
      at: entry.at || nowIso(),
      kind: entry.kind || 'info',
      message: String(entry.message || ''),
    })).filter((entry) => entry.message);
    state.strategyDirty = false;
    if (!options.keepMonitor && state.monitorTimer) {
      clearInterval(state.monitorTimer);
      state.monitorTimer = null;
    }
  }

  function workspacePayload() {
    return {
      version: SCRIPT_VERSION,
      tool: TOOL_MARK,
      exportedAt: nowIso(),
      site: currentSiteInfo(),
      keyPool: state.keyPool,
      activeJob: jobForStorage(state.activeJob),
      workLogs: state.workLogs,
    };
  }

  function persistWorkspaceState() {
    try {
      localStorage.setItem(WORKSPACE_STORAGE_KEY, JSON.stringify(workspacePayload()));
    } catch (err) {
      console.warn('[NewAPI Bulk Channel Importer] workspace persist failed:', err);
    }
  }

  function restoreWorkspaceState() {
    try {
      const raw = localStorage.getItem(WORKSPACE_STORAGE_KEY);
      if (!raw) return;
      applyWorkspacePayload(JSON.parse(raw), { keepMonitor: false });
    } catch (err) {
      console.warn('[NewAPI Bulk Channel Importer] workspace restore failed:', err);
    }
  }

  function readButtonPosition() {
    try {
      const raw = localStorage.getItem(BUTTON_POSITION_KEY);
      if (!raw) return null;
      const position = JSON.parse(raw);
      if (!position || typeof position !== 'object') return null;
      const left = Number(position.left);
      const top = Number(position.top);
      if (!Number.isFinite(left) || !Number.isFinite(top)) return null;
      return { left, top };
    } catch {
      return null;
    }
  }

  function saveButtonPosition(position) {
    try {
      localStorage.setItem(BUTTON_POSITION_KEY, JSON.stringify(position));
    } catch {
      /* ignore storage failures */
    }
  }

  function clampButtonPosition(position, button) {
    const margin = 8;
    const width = button.offsetWidth || 138;
    const height = button.offsetHeight || 68;
    const maxLeft = Math.max(margin, window.innerWidth - width - margin);
    const maxTop = Math.max(margin, window.innerHeight - height - margin);
    return {
      left: Math.min(Math.max(margin, position.left), maxLeft),
      top: Math.min(Math.max(margin, position.top), maxTop),
    };
  }

  function applyButtonPosition(button, position) {
    const clamped = clampButtonPosition(position, button);
    button.style.left = `${clamped.left}px`;
    button.style.top = `${clamped.top}px`;
    button.style.right = 'auto';
    button.style.bottom = 'auto';
    return clamped;
  }

  function restoreButtonPosition(button) {
    const position = readButtonPosition();
    if (!position) return;
    saveButtonPosition(applyButtonPosition(button, position));
  }

  function onButtonPointerDown(event) {
    if (event.button !== undefined && event.button !== 0) return;
    const button = event.currentTarget;
    const rect = button.getBoundingClientRect();
    state.buttonDrag = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      left: rect.left,
      top: rect.top,
      moved: false,
    };
    button.dataset.dragging = 'true';
    button.setPointerCapture?.(event.pointerId);
  }

  function onButtonPointerMove(event) {
    const drag = state.buttonDrag;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const dx = event.clientX - drag.startX;
    const dy = event.clientY - drag.startY;
    if (Math.abs(dx) + Math.abs(dy) > 4) drag.moved = true;
    applyButtonPosition(event.currentTarget, {
      left: drag.left + dx,
      top: drag.top + dy,
    });
  }

  function onButtonPointerEnd(event) {
    const drag = state.buttonDrag;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const button = event.currentTarget;
    try {
      button.releasePointerCapture?.(event.pointerId);
    } catch {
      /* pointer capture may already be released */
    }
    delete button.dataset.dragging;
    if (drag.moved) {
      const rect = button.getBoundingClientRect();
      saveButtonPosition(clampButtonPosition({ left: rect.left, top: rect.top }, button));
      state.buttonClickSuppressedUntil = Date.now() + 350;
    }
    state.buttonDrag = null;
  }

  function setupButtonDrag(button) {
    button.addEventListener('pointerdown', onButtonPointerDown);
    button.addEventListener('pointermove', onButtonPointerMove);
    button.addEventListener('pointerup', onButtonPointerEnd);
    button.addEventListener('pointercancel', onButtonPointerEnd);
    window.addEventListener('resize', () => restoreButtonPosition(button));
  }

  function injectStyles() {
    if (document.getElementById(`${SCRIPT_ID}-style`)) return;
    const style = document.createElement('style');
    style.id = `${SCRIPT_ID}-style`;
    style.textContent = `
      .nai-bulk-button {
        position: fixed;
        right: 22px;
        bottom: 24px;
        z-index: 2147483646;
        min-width: 138px;
        min-height: 68px;
        display: flex;
        flex-direction: column;
        justify-content: center;
        align-items: flex-start;
        gap: 4px;
        overflow: hidden;
        border: 2px solid rgba(255, 190, 158, .95);
        border-radius: 10px;
        background: #e87046;
        color: #1d100a;
        font: 900 15px/1.15 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        padding: 13px 17px;
        box-shadow:
          0 18px 38px rgba(232, 112, 70, .38),
          0 0 0 4px rgba(232, 112, 70, .18),
          inset 0 0 0 1px rgba(29, 16, 10, .3);
        cursor: grab;
        touch-action: none;
        user-select: none;
      }

      .nai-bulk-button::before {
        content: "";
        position: absolute;
        inset: 5px;
        border: 1px solid rgba(255, 238, 226, .42);
        border-radius: 6px;
        pointer-events: none;
      }

      .nai-bulk-button-main,
      .nai-bulk-button-sub {
        position: relative;
        z-index: 1;
      }

      .nai-bulk-button-main {
        letter-spacing: 0;
      }

      .nai-bulk-button-sub {
        font-size: 11px;
        letter-spacing: .08em;
        text-transform: uppercase;
        color: rgba(29, 16, 10, .82);
      }

      .nai-bulk-button:hover {
        background: #f07f55;
        box-shadow:
          0 20px 42px rgba(232, 112, 70, .46),
          0 0 0 5px rgba(232, 112, 70, .22),
          inset 0 0 0 1px rgba(29, 16, 10, .34);
      }

      .nai-bulk-button[data-dragging="true"] {
        cursor: grabbing;
        transform: scale(.98);
      }

      .nai-bulk-panel {
        position: fixed;
        top: 20px;
        right: 20px;
        z-index: 2147483647;
        width: min(1480px, calc(100vw - 40px));
        max-height: calc(100vh - 40px);
        overflow: hidden;
        display: none;
        color: #efeeeb;
        background: #191817;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 10px;
        box-shadow: 0 24px 72px rgba(0, 0, 0, .44);
        font: 14px/1.45 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-bulk-panel[data-open="true"] {
        display: flex;
        flex-direction: column;
      }

      .nai-bulk-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
        padding: 12px 16px;
        border-bottom: 1px solid rgba(255, 255, 255, .1);
      }

      .nai-bulk-title {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 10px;
        min-width: 0;
      }

      .nai-bulk-title strong {
        font-size: 16px;
        letter-spacing: 0;
      }

      .nai-bulk-title-line {
        display: flex;
        align-items: center;
        flex-wrap: wrap;
        gap: 7px;
      }

      .nai-bulk-header-separator {
        width: 1px;
        align-self: stretch;
        min-height: 22px;
        background: rgba(255, 255, 255, .18);
      }

      .nai-bulk-title-badge {
        display: inline-flex;
        align-items: center;
        min-height: 18px;
        padding: 0 7px;
        border: 1px solid rgba(255, 255, 255, .14);
        border-radius: 6px;
        background: #242321;
        color: #d7d4cf;
        font-size: 11px;
        font-weight: 800;
        line-height: 1;
      }

      .nai-bulk-title-badge-mark {
        border-color: rgba(255, 144, 92, .72);
        background: #e87046;
        color: #1d100a;
      }

      .nai-bulk-title > span,
      .nai-bulk-title-site {
        color: #aaa7a1;
        font-size: 12px;
      }

      .nai-bulk-title-site {
        max-width: min(420px, 36vw);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .nai-bulk-title-site strong {
        color: #efeeeb;
        font-size: 12px;
      }

      .nai-bulk-header-actions {
        display: flex;
        align-items: center;
        gap: 8px;
        flex: 0 0 auto;
      }

      .nai-bulk-hidden-file {
        display: none !important;
      }

      .nai-step {
        display: flex;
        flex-direction: column;
        gap: 2px;
      }

      .nai-step-kicker {
        color: #e87046;
        font-size: 11px;
        font-weight: 850;
      }

      .nai-step-title {
        color: #efeeeb;
        font-size: 14px;
        font-weight: 850;
      }

      .nai-step-desc {
        color: #9a9690;
        font-size: 12px;
      }

      .nai-bulk-close,
      .nai-bulk-small-button,
      .nai-bulk-action {
        border: 1px solid rgba(255, 255, 255, .14);
        border-radius: 7px;
        background: #2a2927;
        color: #efeeeb;
        cursor: pointer;
        font: inherit;
      }

      .nai-bulk-close {
        width: 34px;
        height: 34px;
        font-size: 18px;
      }

      .nai-bulk-body {
        overflow: auto;
        padding: 12px;
      }

      .nai-workbench {
        height: calc(100vh - 146px);
        min-height: 620px;
        display: grid;
        grid-template-columns: minmax(340px, .98fr) minmax(360px, 1fr) minmax(420px, 1.18fr);
        gap: 12px;
        align-items: stretch;
      }

      .nai-pane {
        min-width: 0;
        height: 100%;
        max-height: none;
        box-sizing: border-box;
        overflow: auto;
        padding: 12px;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 9px;
        background: #151412;
      }

      .nai-pane-left,
      .nai-pane-center,
      .nai-pane-right {
        display: flex;
        flex-direction: column;
        gap: 12px;
      }

      .nai-pane-left {
        display: grid;
        grid-template-rows: auto auto auto minmax(0, 1fr) auto auto auto;
        overflow: hidden;
      }

      .nai-pane-left > .nai-pane-card {
        grid-row: 5;
      }

      .nai-pane-left > .nai-key-tabs {
        grid-row: 6;
      }

      .nai-pane-left > .nai-key-tab-panel {
        grid-row: 7;
      }

      .nai-pane-center {
        display: grid;
        grid-template-rows: auto auto minmax(0, 1fr) auto;
        overflow: hidden;
      }

      .nai-pane-right {
        background: #181715;
      }

      .nai-right-sticky {
        position: sticky;
        top: -12px;
        z-index: 5;
        display: flex;
        flex-direction: column;
        gap: 10px;
        margin: -12px -12px 0;
        padding: 12px;
        border-bottom: 1px solid rgba(255, 255, 255, .1);
        background: #181715;
      }

      .nai-right-sticky-header {
        display: grid;
        grid-template-columns: minmax(0, 1fr) auto;
        gap: 10px;
        align-items: start;
      }

      .nai-pane-title {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
        color: #efeeeb;
        font-size: 13px;
        font-weight: 800;
      }

      .nai-pane-title small {
        color: #9a9690;
        font-size: 11px;
        font-weight: 650;
      }

      .nai-pane-card {
        padding: 10px;
        border: 1px solid rgba(255, 255, 255, .09);
        border-radius: 8px;
        background: #191817;
      }

      .nai-job-status {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 10px;
      }

      .nai-job-status strong {
        color: #efeeeb;
        font-size: 13px;
      }

      .nai-job-status span {
        color: #9a9690;
        font-size: 12px;
      }

      .nai-job-empty {
        margin-top: 10px;
        padding: 10px;
        border: 1px dashed rgba(255, 255, 255, .14);
        border-radius: 8px;
        background: #141312;
        color: #aaa7a1;
        font-size: 12px;
        line-height: 1.5;
      }

      .nai-job-empty[hidden],
      .nai-job-runtime[hidden] {
        display: none;
      }

      .nai-job-actionbar {
        margin-top: 10px;
        padding: 10px 0 0;
        background: transparent;
      }

      .nai-key-add-actions {
        padding: 0;
        border-top: 0;
        background: transparent;
      }

      .nai-key-add-actions .nai-bulk-action {
        width: 100%;
      }

      .nai-key-summary {
        display: grid;
        grid-template-columns: repeat(3, minmax(0, 1fr));
        gap: 8px;
      }

      .nai-key-tabs {
        display: flex;
        gap: 6px;
        padding: 3px;
        border: 1px solid rgba(255, 255, 255, .09);
        border-radius: 8px;
        background: #11100f;
      }

      .nai-key-tab {
        flex: 1;
        min-height: 32px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: #aaa7a1;
        cursor: pointer;
        font: 750 12px/1 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-key-tab[data-active="true"] {
        background: #2a2927;
        color: #efeeeb;
      }

      .nai-key-tab-panel[hidden] {
        display: none;
      }

      .nai-key-tab-panel {
        min-height: 0;
        height: clamp(260px, 38vh, 430px);
        max-height: none;
        overflow: auto;
      }

      .nai-key-tab-panel:not([hidden]) {
        display: block;
      }

      .nai-key-list {
        min-height: 0;
        height: 100%;
        max-height: none;
        overflow: auto;
        border: 1px solid rgba(255, 255, 255, .08);
        border-radius: 8px;
        background: #11100f;
      }

      .nai-key-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr) 74px;
        gap: 8px;
        padding: 8px 9px;
        border-bottom: 1px solid rgba(255, 255, 255, .07);
        color: #cfcac2;
        font-size: 12px;
      }

      .nai-key-row:last-child {
        border-bottom: 0;
      }

      .nai-key-row strong,
      .nai-key-row span {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .nai-right-placeholder {
        display: none;
        flex: 1;
        height: 100%;
        min-height: 180px;
        align-items: center;
        justify-content: center;
        text-align: center;
        color: #9a9690;
        border: 1px dashed rgba(255, 255, 255, .16);
        border-radius: 8px;
        padding: 18px;
      }

      .nai-bulk-panel[data-nai-right-open="false"] .nai-right-sticky,
      .nai-bulk-panel[data-nai-right-open="false"] .nai-pane-right-form {
        display: none;
      }

      .nai-bulk-panel[data-nai-right-open="false"] .nai-right-placeholder {
        display: flex;
      }

      .nai-bulk-panel[data-nai-right-open="false"] .nai-workbench {
        grid-template-columns: minmax(340px, .98fr) minmax(360px, 1fr) minmax(420px, 1.18fr);
      }

      .nai-bulk-grid {
        display: grid;
        grid-template-columns: repeat(12, 1fr);
        gap: 12px;
      }

      .nai-bulk-field {
        display: flex;
        flex-direction: column;
        gap: 6px;
        min-width: 0;
      }

      .nai-bulk-field label,
      .nai-bulk-check span,
      .nai-bulk-section-title {
        color: #d7d4cf;
        font-weight: 650;
        font-size: 12px;
      }

      .nai-bulk-field small,
      .nai-bulk-help {
        color: #9a9690;
        font-size: 12px;
      }

      .nai-bulk-field input,
      .nai-bulk-field select,
      .nai-bulk-field textarea {
        width: 100%;
        box-sizing: border-box;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 7px;
        background: #242321;
        color: #efeeeb;
        outline: none;
        padding: 9px 10px;
        font: 13px/1.35 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-bulk-hidden-select {
        display: none !important;
      }

      .nai-type-picker {
        position: relative;
      }

      .nai-type-trigger {
        width: 100%;
        min-height: 38px;
        display: flex;
        align-items: center;
        gap: 9px;
        box-sizing: border-box;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 7px;
        background: #242321;
        color: #efeeeb;
        outline: none;
        padding: 8px 10px;
        cursor: pointer;
        font: 13px/1.35 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-type-trigger:hover,
      .nai-type-trigger[aria-expanded="true"] {
        border-color: rgba(232, 112, 70, .72);
        box-shadow: 0 0 0 2px rgba(232, 112, 70, .18);
      }

      .nai-type-trigger::after {
        content: "";
        width: 8px;
        height: 8px;
        margin-left: auto;
        border-right: 2px solid #aaa7a1;
        border-bottom: 2px solid #aaa7a1;
        transform: rotate(45deg) translateY(-2px);
      }

      .nai-type-menu {
        position: absolute;
        top: calc(100% + 6px);
        left: 0;
        right: 0;
        z-index: 2147483647;
        max-height: 292px;
        overflow: auto;
        padding: 6px;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 8px;
        background: #1f1e1c;
        box-shadow: 0 18px 42px rgba(0, 0, 0, .42);
      }

      .nai-type-menu[hidden] {
        display: none;
      }

      .nai-type-option {
        width: 100%;
        display: flex;
        align-items: center;
        gap: 9px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: #efeeeb;
        cursor: pointer;
        padding: 7px 8px;
        text-align: left;
        font: 13px/1.35 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-type-option:hover,
      .nai-type-option:focus {
        background: #2a2927;
        outline: none;
      }

      .nai-type-option[aria-selected="true"] {
        background: rgba(232, 112, 70, .18);
        color: #fff4ee;
      }

      .nai-channel-icon {
        width: 24px;
        height: 24px;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        flex: 0 0 auto;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 7px;
        font: 850 10px/1 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        letter-spacing: .02em;
      }

      .nai-host-channel-icon svg,
      .nai-host-channel-icon img {
        width: 16px;
        height: 16px;
        display: block;
      }

      .nai-type-label {
        min-width: 0;
        flex: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .nai-type-id {
        color: #9a9690;
        font-size: 12px;
        font-weight: 650;
      }

      .nai-bulk-static {
        min-height: 38px;
        display: flex;
        align-items: center;
        box-sizing: border-box;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 7px;
        background: #201f1d;
        color: #d7d4cf;
        padding: 9px 10px;
        font: 700 13px/1.35 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
        overflow-wrap: anywhere;
      }

      .nai-bulk-static-disabled {
        border-color: rgba(255, 255, 255, .08);
        background: #171615;
        color: #85817a;
        cursor: not-allowed;
        opacity: .9;
      }

      .nai-bulk-static[data-empty="true"] {
        color: #aaa7a1;
        font-family: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        font-weight: 650;
      }

      .nai-bulk-static-disabled[data-empty="true"] {
        color: #85817a;
      }

      .nai-bulk-field textarea {
        min-height: 88px;
        resize: vertical;
        font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      }

      .nai-bulk-field input:focus,
      .nai-bulk-field select:focus,
      .nai-bulk-field textarea:focus {
        border-color: rgba(232, 112, 70, .72);
        box-shadow: 0 0 0 2px rgba(232, 112, 70, .18);
      }

      .nai-span-2 { grid-column: span 2; }
      .nai-span-3 { grid-column: span 3; }
      .nai-span-4 { grid-column: span 4; }
      .nai-span-5 { grid-column: span 5; }
      .nai-span-6 { grid-column: span 6; }
      .nai-span-7 { grid-column: span 7; }
      .nai-span-8 { grid-column: span 8; }
      .nai-span-12 { grid-column: span 12; }

      .nai-bulk-section {
        margin-top: 12px;
        padding-top: 12px;
        border-top: 1px solid rgba(255, 255, 255, .1);
      }

      .nai-pane > .nai-bulk-section:first-child,
      .nai-pane-card > .nai-bulk-section:first-child {
        margin-top: 0;
        padding-top: 0;
        border-top: 0;
      }

      .nai-bulk-inline {
        display: flex;
        gap: 8px;
        align-items: end;
      }

      .nai-bulk-inline .nai-bulk-field {
        flex: 1;
      }

      .nai-bulk-combo-row {
        display: flex;
        gap: 8px;
        align-items: center;
      }

      .nai-bulk-combo-row > input,
      .nai-bulk-combo-row > select,
      .nai-bulk-combo-row > .nai-bulk-static {
        flex: 1;
        min-width: 0;
      }

      .nai-bulk-group-panel {
        grid-column: span 12;
        display: block;
        gap: 12px;
        align-items: start;
      }

      .nai-group-picker {
        position: relative;
        flex: 1;
        min-width: 0;
      }

      .nai-group-trigger {
        width: 100%;
        min-height: 38px;
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 8px;
        box-sizing: border-box;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 7px;
        background: #242321;
        color: #efeeeb;
        outline: none;
        padding: 8px 10px;
        cursor: pointer;
        font: 13px/1.35 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-group-trigger:hover,
      .nai-group-trigger[aria-expanded="true"] {
        border-color: rgba(232, 112, 70, .72);
        box-shadow: 0 0 0 2px rgba(232, 112, 70, .18);
      }

      .nai-group-trigger::after {
        content: "";
        width: 8px;
        height: 8px;
        flex: 0 0 auto;
        border-right: 2px solid #aaa7a1;
        border-bottom: 2px solid #aaa7a1;
        transform: rotate(45deg) translateY(-2px);
      }

      .nai-group-trigger-text {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .nai-group-menu {
        position: absolute;
        top: calc(100% + 6px);
        left: 0;
        right: 0;
        z-index: 2147483647;
        max-height: 240px;
        overflow: auto;
        padding: 6px;
        border: 1px solid rgba(255, 255, 255, .12);
        border-radius: 8px;
        background: #1f1e1c;
        box-shadow: 0 18px 42px rgba(0, 0, 0, .42);
      }

      .nai-group-menu[hidden] {
        display: none;
      }

      .nai-group-option {
        width: 100%;
        min-height: 32px;
        display: flex;
        align-items: center;
        gap: 8px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: #efeeeb;
        cursor: pointer;
        padding: 7px 8px;
        text-align: left;
        font: 13px/1.35 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-group-option:hover,
      .nai-group-option:focus {
        background: #2a2927;
        outline: none;
      }

      .nai-group-option[aria-selected="true"] {
        background: rgba(232, 112, 70, .18);
        color: #fff4ee;
      }

      .nai-group-option input {
        width: 15px;
        height: 15px;
        accent-color: #e87046;
        pointer-events: none;
      }

      .nai-bulk-name-builder {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;
        align-items: center;
      }

      .nai-bulk-plus {
        color: #aaa7a1;
        font-weight: 800;
        text-align: center;
      }

      .nai-name-segment {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        min-width: 176px;
      }

      .nai-name-segment-label {
        color: #d7d4cf;
        font-weight: 800;
        font-size: 12px;
      }

      .nai-name-segment select {
        min-width: 132px;
      }

      .nai-name-remove {
        width: 30px;
        height: 34px;
        padding: 0;
      }

      .nai-name-settings {
        margin-top: 10px;
      }

      .nai-name-settings-grid {
        display: grid;
        grid-template-columns: repeat(12, 1fr);
        gap: 10px;
      }

      .nai-name-setting-card {
        grid-column: span 6;
        display: flex;
        flex-direction: column;
        gap: 7px;
        min-width: 0;
        padding: 10px;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 8px;
        background: #171615;
      }

      .nai-name-setting-title {
        color: #d7d4cf;
        font-size: 12px;
        font-weight: 750;
      }

      .nai-name-setting-row {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 8px;
      }

      .nai-template-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr) auto;
        gap: 10px;
        align-items: end;
      }

      .nai-template-label-line {
        display: flex;
        flex-wrap: wrap;
        align-items: baseline;
        gap: 8px;
      }

      .nai-template-actions {
        display: flex;
        gap: 8px;
      }

      .nai-job-controls {
        display: grid;
        grid-template-columns: repeat(12, 1fr);
        gap: 10px;
        align-items: end;
      }

      .nai-job-strategy-row {
        display: grid;
        grid-template-columns: auto repeat(4, minmax(0, 1fr)) auto;
        gap: 8px;
        align-items: end;
      }

      .nai-create-strategy-row {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }

      .nai-create-strategy-row .nai-bulk-check,
      .nai-create-strategy-row .nai-job-strategy-apply {
        grid-column: span 2;
      }

      .nai-job-strategy-field {
        display: grid;
        grid-template-columns: auto minmax(0, 1fr) auto;
        align-items: center;
        gap: 6px;
      }

      .nai-job-strategy-field label {
        color: #aaa7a1;
        font-size: 12px;
        font-weight: 750;
        white-space: nowrap;
      }

      .nai-job-strategy-field input {
        min-height: 34px;
        padding: 7px 8px;
      }

      .nai-job-strategy-unit {
        color: #aaa7a1;
        font-size: 12px;
        font-weight: 650;
      }

      .nai-job-strategy-apply[data-dirty="true"] {
        border-color: #e87046;
        background: #e87046;
        color: #1d100a;
        font-weight: 800;
      }

      .nai-job-preview-card {
        display: grid;
        gap: 8px;
        padding: 10px;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 8px;
        background: #141312;
      }

      .nai-job-preview-row {
        display: grid;
        grid-template-columns: 88px minmax(0, 1fr);
        gap: 8px;
        color: #cfcac2;
        font-size: 12px;
      }

      .nai-job-preview-row span:first-child {
        color: #8f8a82;
        font-weight: 750;
      }

      .nai-job-preview-row strong {
        color: #efeeeb;
        overflow-wrap: anywhere;
      }

      .nai-job-stats {
        display: grid;
        grid-template-columns: repeat(4, minmax(0, 1fr));
        gap: 8px;
        margin-top: 10px;
      }

      .nai-job-stat {
        min-width: 0;
        padding: 9px 10px;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 8px;
        background: #171615;
      }

      .nai-job-stat-label {
        color: #9a9690;
        font-size: 11px;
        font-weight: 650;
      }

      .nai-job-stat-value {
        margin-top: 3px;
        color: #efeeeb;
        font-size: 13px;
        font-weight: 800;
        overflow-wrap: anywhere;
      }

      .nai-job-batches {
        margin-top: 10px;
        max-height: 110px;
        overflow: auto;
        border: 1px solid rgba(255, 255, 255, .08);
        border-radius: 8px;
        background: #11100f;
      }

      .nai-job-batch-row {
        display: grid;
        grid-template-columns: 52px minmax(0, 1fr) 72px 72px;
        gap: 8px;
        padding: 7px 9px;
        border-bottom: 1px solid rgba(255, 255, 255, .07);
        color: #cfcac2;
        font-size: 12px;
      }

      .nai-job-batch-row:last-child {
        border-bottom: 0;
      }

      .nai-job-tabs {
        display: flex;
        gap: 6px;
        padding: 3px;
        border: 1px solid rgba(255, 255, 255, .09);
        border-radius: 8px;
        background: #11100f;
      }

      .nai-job-tab {
        flex: 1;
        min-height: 32px;
        border: 0;
        border-radius: 6px;
        background: transparent;
        color: #aaa7a1;
        cursor: pointer;
        font: 750 12px/1 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }

      .nai-job-tab[data-active="true"] {
        background: #2a2927;
        color: #efeeeb;
      }

      .nai-job-tab-panel[hidden] {
        display: none;
      }

      .nai-pane-center > .nai-pane-card:last-child {
        grid-row: 4;
        height: clamp(330px, 46vh, 500px);
        min-height: 0;
        display: flex;
        flex-direction: column;
        overflow: hidden;
      }

      .nai-job-tab-panel {
        min-height: 0;
        flex: 1 1 auto;
        overflow: hidden;
      }

      .nai-job-tab-panel:not([hidden]) {
        display: flex;
        flex-direction: column;
      }

      #nai-jobStatsPanel .nai-job-stats,
      #nai-jobBatchesPanel .nai-job-batches {
        flex: 1 1 auto;
        min-height: 0;
        overflow: auto;
      }

      #nai-jobStatsPanel .nai-job-stats {
        max-height: none;
        align-content: start;
      }

      #nai-jobBatchesPanel .nai-job-batches {
        max-height: none;
      }

      #nai-jobLogsPanel .nai-bulk-log {
        flex: 1 1 auto;
        min-height: 0;
        max-height: none;
      }

      .nai-bulk-small-button {
        height: 36px;
        padding: 0 10px;
        white-space: nowrap;
      }

      .nai-bulk-small-button:hover,
      .nai-bulk-close:hover,
      .nai-bulk-action:hover {
        background: #34322f;
      }

      .nai-bulk-checks {
        display: flex;
        flex-wrap: wrap;
        gap: 10px 14px;
      }

      .nai-bulk-check {
        display: inline-flex;
        align-items: center;
        gap: 7px;
        min-height: 30px;
      }

      .nai-bulk-check input {
        width: 16px;
        height: 16px;
        accent-color: #e87046;
      }

      .nai-bulk-actions {
        display: flex;
        flex-wrap: wrap;
        gap: 10px;
        padding: 12px 16px;
        border-top: 1px solid rgba(255, 255, 255, .1);
        background: #171615;
      }

      .nai-bulk-action {
        min-height: 38px;
        padding: 0 14px;
      }

      .nai-bulk-action-primary {
        background: #e87046;
        color: #1d100a;
        border-color: #e87046;
        font-weight: 700;
      }

      .nai-bulk-action-primary:hover {
        background: #f07f55;
      }

      .nai-bulk-action:disabled,
      .nai-bulk-small-button:disabled {
        opacity: .52;
        cursor: not-allowed;
      }

      .nai-bulk-preview {
        max-height: 220px;
        overflow: auto;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 8px;
      }

      .nai-bulk-preview table {
        width: 100%;
        border-collapse: collapse;
        font-size: 12px;
      }

      .nai-bulk-preview th,
      .nai-bulk-preview td {
        text-align: left;
        padding: 8px 9px;
        border-bottom: 1px solid rgba(255, 255, 255, .08);
        vertical-align: top;
      }

      .nai-bulk-preview th {
        position: sticky;
        top: 0;
        background: #20201e;
        color: #cfcac2;
      }

      .nai-bulk-log {
        min-height: 92px;
        max-height: 180px;
        overflow: auto;
        white-space: pre-wrap;
        border: 1px solid rgba(255, 255, 255, .1);
        border-radius: 8px;
        background: #11100f;
        color: #c9d6d0;
        padding: 10px;
        font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      }

      .nai-bulk-muted {
        color: #aaa7a1;
      }

      .nai-bulk-ok { color: #77d1a1; }
      .nai-bulk-error { color: #ff9a8c; }

      details.nai-bulk-details summary {
        cursor: pointer;
        color: #d7d4cf;
        font-weight: 650;
        font-size: 12px;
        margin-bottom: 10px;
      }

      @media (max-width: 720px) {
        .nai-bulk-panel {
          top: 10px;
          right: 10px;
          width: calc(100vw - 20px);
          max-height: calc(100vh - 20px);
        }
        .nai-workbench,
        .nai-bulk-panel[data-nai-right-open="false"] .nai-workbench {
          grid-template-columns: 1fr;
          height: auto;
          min-height: 0;
        }
        .nai-pane {
          height: auto;
          max-height: none;
        }
        .nai-pane-left,
        .nai-pane-center {
          display: flex;
          overflow: auto;
        }
        .nai-bulk-group-panel,
        .nai-template-row,
        .nai-job-controls,
        .nai-job-strategy-row,
        .nai-name-setting-row {
          grid-template-columns: 1fr;
        }
        .nai-span-2,
        .nai-span-3,
        .nai-span-4,
        .nai-span-5,
        .nai-span-6,
        .nai-span-7,
        .nai-span-8 {
          grid-column: span 12;
        }
        .nai-bulk-name-builder {
          flex-direction: column;
          align-items: stretch;
        }
        .nai-bulk-plus {
          display: none;
        }
        .nai-name-segment,
        .nai-name-segment select,
        .nai-name-setting-card {
          width: 100%;
          grid-column: span 12;
        }
        .nai-job-stats {
          grid-template-columns: 1fr 1fr;
        }
        .nai-job-batch-row {
          grid-template-columns: 44px minmax(0, 1fr) 60px;
        }
        .nai-job-batch-row span:nth-child(4) {
          display: none;
        }
        .nai-template-actions {
          justify-content: flex-start;
        }
      }
    `;
    document.head.appendChild(style);
  }

  function renderTypeOptions(selected) {
    const options = CHANNEL_TYPES.map(([value, label]) => {
      const isSelected = String(value) === String(selected) ? ' selected' : '';
      return `<option value="${value}"${isSelected}>${escapeHtml(label)} (${value})</option>`;
    });
    return options.join('');
  }

  function channelTypeEntry(type) {
    const entry = CHANNEL_TYPES.find(([value]) => String(value) === String(type));
    if (entry) return entry;
    return [Number(type) || 0, `#${type}`];
  }

  function channelTypeIconName(type) {
    return CHANNEL_TYPE_ICONS[Number(type)] || 'OpenAI';
  }

  function sanitizeHostIcon(node) {
    if (!node) return '';
    const clone = node.cloneNode(true);
    qsa('script, style', clone).forEach((child) => child.remove());
    qsa('*', clone).forEach((child) => {
      for (const attr of Array.from(child.attributes)) {
        if (/^on/i.test(attr.name)) child.removeAttribute(attr.name);
      }
    });
    for (const attr of Array.from(clone.attributes || [])) {
      if (/^on/i.test(attr.name)) clone.removeAttribute(attr.name);
    }
    const tag = clone.tagName?.toLowerCase();
    if (tag === 'img') {
      const src = clone.getAttribute('src') || '';
      if (!src || /^javascript:/i.test(src)) return '';
      return `<img src="${escapeHtml(src)}" alt="">`;
    }
    if (tag === 'svg') return clone.outerHTML;
    return '';
  }

  function captureHostTypeIcons() {
    const panel = document.getElementById(SCRIPT_ID);
    const candidates = qsa('button, [role="option"], [role="menuitem"], [class*="select"], [class*="channel"], [class*="provider"]')
      .filter((el) => !panel?.contains(el));
    const icons = {};
    for (const [type, label] of CHANNEL_TYPES) {
      const normalized = String(label).toLowerCase();
      const match = candidates.find((el) => {
        const text = String(el.textContent || '').toLowerCase();
        return text.includes(normalized) && qs('svg, img', el);
      });
      const icon = sanitizeHostIcon(match ? qs('svg, img', match) : null);
      if (icon) icons[type] = icon;
    }
    state.hostTypeIcons = icons;
  }

  function channelIconHtml(type) {
    const hostIcon = state.hostTypeIcons?.[Number(type)];
    if (hostIcon) {
      return `
        <span class="nai-channel-icon nai-host-channel-icon" title="NewAPI ${escapeHtml(channelTypeIconName(type))}" aria-hidden="true">${hostIcon}</span>
      `;
    }
    const iconName = channelTypeIconName(type);
    const [text, background, color] = CHANNEL_ICON_META[iconName] || CHANNEL_ICON_META.OpenAI;
    return `
      <span
        class="nai-channel-icon"
        title="${escapeHtml(iconName)}"
        style="background:${escapeHtml(background)};color:${escapeHtml(color)};"
        aria-hidden="true"
      >${escapeHtml(text)}</span>
    `;
  }

  function typeOptionContentHtml(type, label) {
    return `
      ${channelIconHtml(type)}
      <span class="nai-type-label">${escapeHtml(label)}</span>
      <span class="nai-type-id">${escapeHtml(type)}</span>
    `;
  }

  function renderTypePickerValue(selected) {
    const [type, label] = channelTypeEntry(selected);
    return typeOptionContentHtml(type, label);
  }

  function renderTypeMenuOptions(selected) {
    return CHANNEL_TYPES.map(([type, label]) => {
      const isSelected = String(type) === String(selected);
      return `
        <button
          type="button"
          class="nai-type-option"
          data-nai-type-option="${escapeHtml(type)}"
          role="option"
          aria-selected="${isSelected ? 'true' : 'false'}"
        >${typeOptionContentHtml(type, label)}</button>
      `;
    }).join('');
  }

  function renderNameSegmentTypeOptions(selected) {
    return NAME_SEGMENT_TYPES
      .map(([value, label]) => {
        const isSelected = String(value) === String(selected) ? ' selected' : '';
        return `<option value="${escapeHtml(value)}"${isSelected}>${escapeHtml(label)}</option>`;
      })
      .join('');
  }

  function nameSegmentLabel(type) {
    const found = NAME_SEGMENT_TYPES.find(([value]) => value === type);
    return found ? found[1] : '空';
  }

  function renderNameBuilderHtml(config) {
    const segments = normalizeNameSegments(config.nameSegments, config);
    return `
      <div class="nai-bulk-name-builder" data-nai-name-builder>
        ${segments.map((type, index) => `
          <div class="nai-name-segment" data-nai-name-segment="${index}">
            <span class="nai-name-segment-label">${escapeHtml(slotLabel(index))}:</span>
            <select data-nai-name-segment-type="${index}" aria-label="名称段 ${escapeHtml(slotLabel(index))}">
              ${renderNameSegmentTypeOptions(type)}
            </select>
            ${segments.length > 1 ? `<button type="button" class="nai-bulk-small-button nai-name-remove" data-nai-name-remove="${index}" title="删除此段">x</button>` : ''}
          </div>
          ${index < segments.length - 1 ? '<span class="nai-bulk-plus">+</span>' : ''}
        `).join('')}
        ${segments.length < MAX_NAME_SEGMENTS ? '<button type="button" class="nai-bulk-small-button" data-nai-name-add-segment>+ 添加段</button>' : ''}
      </div>
    `;
  }

  function settingValue(settings, slot, key) {
    return settings[slot]?.[key] ?? defaultSegmentSettings(slot)[key] ?? '';
  }

  function settingInput(slot, key, label, value, attrs = '') {
    return `
      <label class="nai-bulk-field">
        <span>${escapeHtml(label)}</span>
        <input data-nai-name-setting="${escapeHtml(slot)}.${escapeHtml(key)}" value="${escapeHtml(value)}" ${attrs}>
      </label>
    `;
  }

  function renderNameSegmentSettingsHtml(config) {
    const segments = normalizeNameSegments(config.nameSegments, config);
    const settings = normalizeNameSegmentSettings(config.nameSegmentSettings, config);
    const cards = segments
      .map((type, index) => {
        const slot = slotLabel(index);
        if (type === 'text') {
          return `
            <div class="nai-name-setting-card">
              <div class="nai-name-setting-title">固定文字.${escapeHtml(slot)}</div>
              ${settingInput(slot, 'text', '自定义', settingValue(settings, slot, 'text'))}
            </div>
          `;
        }
        if (type === 'num') {
          return `
            <div class="nai-name-setting-card">
              <div class="nai-name-setting-title">顺序数字.${escapeHtml(slot)}</div>
              <div class="nai-name-setting-row">
                ${settingInput(slot, 'numberStart', '起始', settingValue(settings, slot, 'numberStart'), 'inputmode="numeric"')}
                ${settingInput(slot, 'numberPad', '位数', settingValue(settings, slot, 'numberPad'), 'inputmode="numeric"')}
              </div>
            </div>
          `;
        }
        if (type === 'alpha') {
          return `
            <div class="nai-name-setting-card">
              <div class="nai-name-setting-title">顺序字母.${escapeHtml(slot)}</div>
              ${settingInput(slot, 'alphaStart', '字母起始', settingValue(settings, slot, 'alphaStart'))}
            </div>
          `;
        }
        if (type === 'ts') {
          return `
            <div class="nai-name-setting-card">
              <div class="nai-name-setting-title">时间戳.${escapeHtml(slot)}</div>
              ${settingInput(slot, 'tsFormat', '格式', settingValue(settings, slot, 'tsFormat'), 'placeholder="yyyyMMdd-HHmmss"')}
            </div>
          `;
        }
        if (type === 'date') {
          return `
            <div class="nai-name-setting-card">
              <div class="nai-name-setting-title">日期.${escapeHtml(slot)}</div>
              ${settingInput(slot, 'dateFormat', '格式', settingValue(settings, slot, 'dateFormat'), 'placeholder="yyyyMMdd"')}
            </div>
          `;
        }
        return '';
      })
      .filter(Boolean);

    if (!cards.length) {
      return '<div class="nai-name-settings" data-nai-name-settings></div>';
    }

    return `
      <div class="nai-name-settings" data-nai-name-settings>
        <div class="nai-name-settings-grid">${cards.join('')}</div>
      </div>
    `;
  }

  function selectedGroupsFromValue(value, groups = []) {
    const selected = normalizeList(value).split(',').filter(Boolean);
    if (!selected.length) return [];
    return selected.filter((group) => groups.includes(group));
  }

  function renderGroupOptions(groups = [], selected = '') {
    if (!groups.length) {
      return '<option value="">暂无分组</option>';
    }
    const selectedGroups = new Set(selectedGroupsFromValue(selected, groups));
    return groups.map((group) => {
      const isSelected = selectedGroups.has(group) ? ' selected' : '';
      return `<option value="${escapeHtml(group)}"${isSelected}>${escapeHtml(group)}</option>`;
    }).join('');
  }

  function renderGroupMenuOptions(groups = [], selected = '') {
    if (!groups.length) {
      return '<div class="nai-bulk-help" style="padding: 8px;">暂无分组</div>';
    }
    const selectedGroups = new Set(selectedGroupsFromValue(selected, groups));
    return groups.map((group) => {
      const isSelected = selectedGroups.has(group);
      return `
        <button type="button" class="nai-group-option" data-nai-group-option="${escapeHtml(group)}" aria-selected="${isSelected ? 'true' : 'false'}">
          <input type="checkbox" tabindex="-1"${isSelected ? ' checked' : ''}>
          <span>${escapeHtml(group)}</span>
        </button>
      `;
    }).join('');
  }

  function groupTriggerLabel(value) {
    const groups = normalizeList(value).split(',').filter(Boolean);
    if (!groups.length) return '选择分组';
    if (groups.length <= 2) return groups.join(', ');
    return `${groups.slice(0, 2).join(', ')} +${groups.length - 2}`;
  }

  function currentSiteInfo() {
    const title = String(document.title || '').replace(/\s+/g, ' ').trim();
    return {
      name: title || location.hostname,
      url: location.origin,
    };
  }

  function defaultBaseUrlForType(type) {
    return CHANNEL_BASE_URLS[Number(type)] || '';
  }

  function baseUrlDisplayValue(type) {
    return defaultBaseUrlForType(type) || '此类型无内置默认 Base URL';
  }

  function renderTemplateOptions(channels = [], selected = '') {
    if (!channels.length) {
      return '<option value="">暂无同类型样板渠道</option>';
    }
    return [
      '<option value="">选择样板渠道</option>',
      ...channels.map((channel) => {
        const modelCount = normalizeList(channel.models).split(',').filter(Boolean).length;
        const label = `#${channel.id} ${channel.name || '(未命名)'}${modelCount ? ` - ${modelCount} 模型` : ''}`;
        const isSelected = String(channel.id) === String(selected) ? ' selected' : '';
        return `<option value="${escapeHtml(channel.id)}"${isSelected}>${escapeHtml(label)}</option>`;
      }),
    ].join('');
  }

  function panelHtml(config) {
    const site = currentSiteInfo();
    const defaultBaseUrl = defaultBaseUrlForType(config.typePreset);
    return `
      <div class="nai-bulk-header">
        <div class="nai-bulk-title">
          <strong class="nai-bulk-title-line">NewAPI 批量添加渠道</strong>
          <span class="nai-bulk-header-separator"></span>
          ${versionBadgeHtml()}
          <span class="nai-bulk-header-separator"></span>
          <span>直接调用 /api/channel，兼容 v0.13.2 和 v1.x 登录态。</span>
          <span class="nai-bulk-header-separator"></span>
          <span class="nai-bulk-title-site">当前站点：<strong id="nai-siteName">${escapeHtml(site.name)}</strong> · <strong id="nai-siteUrl">${escapeHtml(site.url)}</strong></span>
        </div>
        <div class="nai-bulk-header-actions">
          <button type="button" class="nai-bulk-small-button" data-nai-refresh-site>刷新站点</button>
          <button type="button" class="nai-bulk-small-button" data-nai-import-work>导入工作</button>
          <button type="button" class="nai-bulk-small-button" data-nai-export-work>导出工作</button>
          <button type="button" class="nai-bulk-small-button" data-nai-reset-work>重置</button>
          <button type="button" class="nai-bulk-close" data-nai-close aria-label="关闭">x</button>
          <input class="nai-bulk-hidden-file" type="file" accept="application/json,.json" data-nai-import-work-file>
        </div>
      </div>
      <div class="nai-bulk-body">
        <div class="nai-workbench">
          <aside class="nai-pane nai-pane-left">
            <div class="nai-step">
              <div class="nai-step-kicker">第一步</div>
              <div class="nai-step-title">批量添加 key</div>
              <div class="nai-step-desc">粘贴后点击添加入库；key 池会保留历史，直到你点击顶栏重置。</div>
            </div>
            <div class="nai-bulk-field">
              <label for="nai-keys">key 库，批量粘贴/追加</label>
              <textarea id="nai-keys" data-nai-sensitive placeholder="sk-ant-...&#10;或 JSON 数组、逗号分隔、key=value、带序号/引号的列表&#10;&#10;入库后会进入当前工作记录；导出工作会包含 key 池，用于迁移后继续执行。"></textarea>
            </div>
            <div class="nai-bulk-actions nai-key-add-actions">
              <button type="button" class="nai-bulk-action nai-bulk-action-primary" data-nai-add-keys>添加入库</button>
            </div>

            <div class="nai-pane-card">
              <div class="nai-pane-title">
                <span>key 池信息</span>
                <small id="nai-keyPoolUpdated">未入库</small>
              </div>
              <div id="nai-keyPoolSummary" class="nai-key-summary"></div>
            </div>

            <div class="nai-key-tabs" data-nai-key-tabs>
              <button type="button" class="nai-key-tab" data-nai-key-tab="list" data-active="true">key 库列表</button>
              <button type="button" class="nai-key-tab" data-nai-key-tab="stats" data-active="false">统计信息</button>
            </div>
            <div id="nai-keyListPanel" class="nai-key-tab-panel"></div>
            <div id="nai-keyStatsPanel" class="nai-key-tab-panel" hidden></div>
          </aside>

          <main class="nai-pane nai-pane-center">
            <div class="nai-step">
              <div class="nai-step-kicker">第二步</div>
              <div class="nai-step-title">添加作业参数</div>
              <div class="nai-step-desc">右侧只用于新建和预览作业；已创建的作业不直接编辑，变更配置会按新输入新建作业。</div>
            </div>

            <div class="nai-pane-card">
              <div class="nai-job-status">
                <div>
                  <strong id="nai-jobTitle">暂无作业</strong>
                  <span id="nai-jobStatusText">未开始</span>
                </div>
                <button type="button" class="nai-bulk-small-button" data-nai-open-params>创建作业参数</button>
              </div>
              <div id="nai-jobEmptyState" class="nai-job-empty">
                尚未创建作业。先点击“创建作业参数”，在右侧配置作业名称、类型、模型、分组、名称组合和策略，然后点击“保存创建作业”。
              </div>
              <div id="nai-jobRuntimeSection" class="nai-bulk-section nai-job-runtime" hidden>
                <div class="nai-job-strategy-row">
                  <label class="nai-bulk-check">
                    <input id="nai-runtime-autoRefill" type="checkbox" data-nai-runtime-check="autoRefill"${config.autoRefill ? ' checked' : ''}>
                    <span>自动</span>
                  </label>
                  <div class="nai-job-strategy-field">
                    <label for="nai-runtime-targetAliveSize">保活</label>
                    <input id="nai-runtime-targetAliveSize" data-nai-runtime-field="targetAliveSize" inputmode="numeric" value="${escapeHtml(config.targetAliveSize)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-runtime-aliveThreshold">低于</label>
                    <input id="nai-runtime-aliveThreshold" data-nai-runtime-field="aliveThreshold" inputmode="numeric" value="${escapeHtml(config.aliveThreshold)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-runtime-replenishBatchSize">添加</label>
                    <input id="nai-runtime-replenishBatchSize" data-nai-runtime-field="replenishBatchSize" inputmode="numeric" value="${escapeHtml(config.replenishBatchSize)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-runtime-monitorIntervalSec">监控间隔</label>
                    <input id="nai-runtime-monitorIntervalSec" data-nai-runtime-field="monitorIntervalSec" inputmode="numeric" value="${escapeHtml(config.monitorIntervalSec)}">
                    <span class="nai-job-strategy-unit">秒</span>
                  </div>
                  <button type="button" class="nai-bulk-small-button nai-job-strategy-apply" data-nai-apply-strategy data-dirty="false" disabled>应用策略</button>
                </div>
              </div>
              <div class="nai-bulk-actions nai-job-actionbar">
                <button type="button" class="nai-bulk-action nai-bulk-action-primary" data-nai-toggle-job>开启/暂停作业</button>
                <button type="button" class="nai-bulk-action" data-nai-refresh-job>刷新状态</button>
              </div>
            </div>

            <div class="nai-pane-card">
              <div class="nai-job-tabs" data-nai-job-tabs>
                <button type="button" class="nai-job-tab" data-nai-job-tab="stats" data-active="true">作业统计</button>
                <button type="button" class="nai-job-tab" data-nai-job-tab="batches" data-active="false">批次记录</button>
                <button type="button" class="nai-job-tab" data-nai-job-tab="logs" data-active="false">作业日志</button>
              </div>
              <div id="nai-jobStatsPanel" class="nai-job-tab-panel">
                <div class="nai-pane-title" style="margin-top: 10px;">
                  <span>作业统计</span>
                  <small>监控中即时刷新</small>
                </div>
                <div id="nai-jobStats" class="nai-job-stats"></div>
              </div>
              <div id="nai-jobBatchesPanel" class="nai-job-tab-panel" hidden>
                <div class="nai-pane-title" style="margin-top: 10px;">
                  <span>批次记录</span>
                  <small>作业补批和创建批次</small>
                </div>
                <div id="nai-jobBatches" class="nai-job-batches"></div>
              </div>
              <div id="nai-jobLogsPanel" class="nai-job-tab-panel" hidden>
                <div class="nai-pane-title" style="margin-top: 10px;">
                  <span>作业日志</span>
                  <button type="button" class="nai-bulk-small-button" data-nai-export-job>导出日志</button>
                </div>
                <div id="nai-log" class="nai-bulk-log">Ready.</div>
              </div>
            </div>
          </main>

          <aside class="nai-pane nai-pane-right">
            <div class="nai-right-placeholder">右侧用于创建或预览作业参数。点击中栏“创建作业参数”后显示。</div>
            <div class="nai-right-sticky">
              <div class="nai-right-sticky-header">
                <div class="nai-step">
                  <div class="nai-step-kicker">创建 / 预览</div>
                  <div class="nai-step-title">作业信息</div>
                  <div class="nai-step-desc">这里用于新建作业和查看当前作业快照；已创建作业不直接编辑。</div>
                </div>
                <button type="button" class="nai-bulk-action nai-bulk-action-primary" data-nai-run>保存创建</button>
              </div>
              <div id="nai-jobPreview" class="nai-job-preview-card"></div>
            </div>
            <div class="nai-pane-right-form">
              <div class="nai-bulk-field">
                <label for="nai-jobName">自定义作业名称</label>
                <input id="nai-jobName" data-nai-field="jobName" placeholder="留空自动使用 时间戳+类型" value="${escapeHtml(config.jobName)}">
                <small>如果不填写，创建时自动补充为“时间戳 + 类型”。已有作业不会被改名；再次创建会生成新作业。</small>
              </div>
              <div class="nai-bulk-grid">
                <div class="nai-bulk-field nai-span-4">
                  <label>API 路径</label>
                  <div class="nai-bulk-static">${escapeHtml(API_ROOT)}</div>
                </div>
                <div class="nai-bulk-field nai-span-8">
                  <label for="nai-typePreset">类型</label>
                  <div class="nai-type-picker" data-nai-type-picker>
                    <button
                      type="button"
                      class="nai-type-trigger"
                      data-nai-type-trigger
                      aria-haspopup="listbox"
                      aria-expanded="false"
                    >${renderTypePickerValue(config.typePreset)}</button>
                    <div class="nai-type-menu" data-nai-type-menu role="listbox" hidden>
                      ${renderTypeMenuOptions(config.typePreset)}
                    </div>
                  </div>
                  <select id="nai-typePreset" class="nai-bulk-hidden-select" data-nai-field="typePreset" aria-hidden="true" tabindex="-1">${renderTypeOptions(config.typePreset)}</select>
                </div>
                <div class="nai-bulk-group-panel">
                  <div class="nai-bulk-field">
                    <label for="nai-groupTrigger">分组选择</label>
                    <div class="nai-bulk-combo-row">
                      <div class="nai-group-picker" data-nai-group-picker>
                        <button type="button" id="nai-groupTrigger" class="nai-group-trigger" data-nai-group-trigger aria-haspopup="listbox" aria-expanded="false">
                          <span class="nai-group-trigger-text" data-nai-group-trigger-text>${escapeHtml(groupTriggerLabel(config.group))}</span>
                        </button>
                        <div class="nai-group-menu" data-nai-group-menu role="listbox" hidden>${renderGroupMenuOptions([], config.group)}</div>
                        <select id="nai-groupSelect" class="nai-bulk-hidden-select" data-nai-group-select multiple aria-hidden="true" tabindex="-1">${renderGroupOptions([], config.group)}</select>
                      </div>
                      <button type="button" class="nai-bulk-small-button" data-nai-refresh-groups>刷新</button>
                    </div>
                    <input type="hidden" id="nai-group" data-nai-field="group" value="${escapeHtml(config.group)}">
                    <small id="nai-group-help">已读取的分组会在下拉中展示；可多选，已选项直接显示在分组选择框内。</small>
                  </div>
                </div>
                <div class="nai-bulk-field nai-span-12">
                  <label>API 地址（NewAPI 内置默认，仅展示）</label>
                  <div class="nai-bulk-combo-row">
                    <div id="nai-baseUrlDisplay" class="nai-bulk-static nai-bulk-static-disabled" data-empty="${defaultBaseUrl ? 'false' : 'true'}" aria-disabled="true">${escapeHtml(baseUrlDisplayValue(config.typePreset))}</div>
                    <button type="button" class="nai-bulk-small-button" data-nai-refresh-base-url>刷新</button>
                  </div>
                  <small>创建时不会填写 base_url；提交会留空，让 NewAPI 使用内置默认地址。</small>
                </div>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-bulk-field">
                  <label>名称组合</label>
                  <div id="nai-nameBuilderHost">${renderNameBuilderHtml(config)}</div>
                  <small>A/B/C 按顺序拼接；点“+ 添加段”继续扩展。</small>
                  <div id="nai-nameSettingsHost">${renderNameSegmentSettingsHtml(config)}</div>
                </div>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-template-row">
                  <div class="nai-bulk-field">
                    <div class="nai-template-label-line">
                      <label for="nai-templateSelect">选择样板渠道</label>
                      <small id="nai-template-help">*按当前类型读取最近 ${TEMPLATE_PAGE_SIZE} 个渠道。</small>
                    </div>
                    <select id="nai-templateSelect" data-nai-template-select>${renderTemplateOptions()}</select>
                  </div>
                  <div class="nai-template-actions">
                    <button type="button" class="nai-bulk-small-button" data-nai-load-template>读取样板</button>
                    <button type="button" class="nai-bulk-small-button" data-nai-refresh-templates>刷新样板</button>
                  </div>
                </div>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-bulk-grid">
                  <div class="nai-bulk-field nai-span-12">
                    <label for="nai-models">模型</label>
                    <textarea id="nai-models" data-nai-field="models" placeholder="claude-sonnet-4-20250514,claude-opus-4-20250514">${escapeHtml(config.models)}</textarea>
                    <small>支持逗号或换行；提交前会去重并转换成逗号分隔。</small>
                  </div>
                  <div class="nai-bulk-field nai-span-12">
                    <label for="nai-modelMapping">模型映射 JSON</label>
                    <textarea id="nai-modelMapping" data-nai-field="modelMapping" placeholder='{"claude-sonnet-4": "claude-sonnet-4-20250514"}'>${escapeHtml(config.modelMapping)}</textarea>
                    <small>留空表示不配置。NewAPI 要求值必须是字符串。</small>
                  </div>
                </div>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-bulk-grid">
                  <div class="nai-bulk-field nai-span-3">
                    <label for="nai-priority">优先级</label>
                    <input id="nai-priority" data-nai-field="priority" inputmode="numeric" value="${escapeHtml(config.priority)}">
                  </div>
                  <div class="nai-bulk-field nai-span-3">
                    <label for="nai-weight">权重</label>
                    <input id="nai-weight" data-nai-field="weight" inputmode="numeric" value="${escapeHtml(config.weight)}">
                  </div>
                  <div class="nai-bulk-field nai-span-3">
                    <label for="nai-delayMs">间隔 ms</label>
                    <input id="nai-delayMs" data-nai-field="delayMs" inputmode="numeric" value="${escapeHtml(config.delayMs)}">
                  </div>
                  <div class="nai-bulk-field nai-span-3">
                    <label for="nai-tag">标签</label>
                    <input id="nai-tag" data-nai-field="tag" value="${escapeHtml(config.tag)}">
                  </div>
                  <div class="nai-bulk-field nai-span-12">
                    <label for="nai-remark">备注</label>
                    <input id="nai-remark" data-nai-field="remark" value="${escapeHtml(config.remark)}">
                  </div>
                  <div class="nai-span-12 nai-bulk-checks">
                    ${checkboxHtml('status', '启用', config.status)}
                    ${checkboxHtml('autoBan', '自动禁用', config.autoBan)}
                    ${checkboxHtml('dedupeKeys', 'key 去重', config.dedupeKeys)}
                    ${checkboxHtml('continueOnError', '遇错继续', config.continueOnError)}
                    ${checkboxHtml('allowServiceTier', '允许 service_tier', config.allowServiceTier)}
                    ${checkboxHtml('allowInferenceGeo', '允许 inference_geo', config.allowInferenceGeo)}
                    ${checkboxHtml('allowSpeed', '允许 speed', config.allowSpeed)}
                    ${checkboxHtml('claudeBetaQuery', 'Claude beta query', config.claudeBetaQuery)}
                  </div>
                </div>
              </div>

              <div class="nai-bulk-section">
                <details class="nai-bulk-details">
                  <summary>高级 JSON 字段</summary>
                  <div class="nai-bulk-grid">
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-settingJson">setting JSON</label>
                      <textarea id="nai-settingJson" data-nai-field="settingJson">${escapeHtml(config.settingJson)}</textarea>
                    </div>
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-settingsJson">settings JSON</label>
                      <textarea id="nai-settingsJson" data-nai-field="settingsJson">${escapeHtml(config.settingsJson)}</textarea>
                    </div>
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-paramOverride">param_override JSON</label>
                      <textarea id="nai-paramOverride" data-nai-field="paramOverride">${escapeHtml(config.paramOverride)}</textarea>
                    </div>
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-headerOverride">header_override JSON</label>
                      <textarea id="nai-headerOverride" data-nai-field="headerOverride">${escapeHtml(config.headerOverride)}</textarea>
                    </div>
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-statusCodeMapping">status_code_mapping JSON</label>
                      <textarea id="nai-statusCodeMapping" data-nai-field="statusCodeMapping">${escapeHtml(config.statusCodeMapping)}</textarea>
                    </div>
                    <div class="nai-bulk-field nai-span-6">
                      <label for="nai-other">other</label>
                      <textarea id="nai-other" data-nai-field="other">${escapeHtml(config.other)}</textarea>
                    </div>
                  </div>
                </details>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-bulk-section-title">策略</div>
                <div class="nai-job-strategy-row nai-create-strategy-row">
                  <label class="nai-bulk-check">
                    <input id="nai-create-autoRefill" type="checkbox" data-nai-check="autoRefill"${config.autoRefill ? ' checked' : ''}>
                    <span>自动补货</span>
                  </label>
                  <div class="nai-job-strategy-field">
                    <label for="nai-create-targetAliveSize">保活</label>
                    <input id="nai-create-targetAliveSize" data-nai-field="targetAliveSize" inputmode="numeric" value="${escapeHtml(config.targetAliveSize)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-create-aliveThreshold">低于</label>
                    <input id="nai-create-aliveThreshold" data-nai-field="aliveThreshold" inputmode="numeric" value="${escapeHtml(config.aliveThreshold)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-create-replenishBatchSize">添加</label>
                    <input id="nai-create-replenishBatchSize" data-nai-field="replenishBatchSize" inputmode="numeric" value="${escapeHtml(config.replenishBatchSize)}">
                  </div>
                  <div class="nai-job-strategy-field">
                    <label for="nai-create-monitorIntervalSec">间隔</label>
                    <input id="nai-create-monitorIntervalSec" data-nai-field="monitorIntervalSec" inputmode="numeric" value="${escapeHtml(config.monitorIntervalSec)}">
                    <span class="nai-job-strategy-unit">秒</span>
                  </div>
                </div>
                <small class="nai-bulk-muted">创建作业时使用这里的策略；创建后会复制到中栏作业第二行，可随时修改并应用。</small>
              </div>

              <div class="nai-bulk-section">
                <div class="nai-bulk-section-title">预览</div>
                <div id="nai-preview" class="nai-bulk-preview"></div>
              </div>
              <div class="nai-bulk-section nai-bulk-actions">
                <button type="button" class="nai-bulk-action" data-nai-preview>刷新预览</button>
                <button type="button" class="nai-bulk-action" data-nai-copy-payload>复制首条 payload</button>
                <button type="button" class="nai-bulk-action nai-bulk-action-primary" data-nai-run>保存创建作业</button>
              </div>
            </div>
          </aside>
        </div>
      </div>
    `;
  }

  function checkboxHtml(id, label, checked) {
    return `
      <label class="nai-bulk-check">
        <input id="nai-${id}" type="checkbox" data-nai-check="${id}"${checked ? ' checked' : ''}>
        <span>${escapeHtml(label)}</span>
      </label>
    `;
  }

  function versionBadgeHtml() {
    return `
      <span class="nai-bulk-title-badge">v${escapeHtml(SCRIPT_VERSION)}</span>
      <span class="nai-bulk-header-separator"></span>
      <span class="nai-bulk-title-badge nai-bulk-title-badge-mark">${escapeHtml(TOOL_MARK)}</span>
    `;
  }

  function mount() {
    if (document.getElementById(SCRIPT_ID)) return;
    injectStyles();
    captureHostTypeIcons();
    restoreWorkspaceState();

    const config = loadConfig();
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'nai-bulk-button';
    button.setAttribute('aria-label', `NewAPI 批量添加渠道 ${TOOL_MARK} v${SCRIPT_VERSION}`);
    button.title = '拖动调整位置，点击打开';
    button.innerHTML = `
      <span class="nai-bulk-button-main">批量渠道</span>
      <span class="nai-bulk-button-sub">${escapeHtml(TOOL_MARK)} v${escapeHtml(SCRIPT_VERSION)}</span>
    `;
    button.addEventListener('click', (event) => {
      if (Date.now() < state.buttonClickSuppressedUntil) {
        event.preventDefault();
        return;
      }
      togglePanel(true);
    });
    setupButtonDrag(button);

    const panel = document.createElement('div');
    panel.id = SCRIPT_ID;
    panel.className = 'nai-bulk-panel';
    panel.setAttribute('data-open', 'false');
    panel.setAttribute('data-nai-right-open', String(Boolean(state.activeJob)));
    panel.innerHTML = panelHtml(config);

    document.body.append(button, panel);
    restoreButtonPosition(button);
    bindPanel(panel);
    setupTypePicker(panel);
    updateBaseUrlDisplay();
    refreshPreview();
    renderWorkLog();
    updateJobStats();
    updateJobControls();
    loadGroups();
    loadTemplates();
    if (state.activeJob && !state.activeJob.stopped && !state.activeJob.paused) {
      startMonitorLoop();
    }
  }

  function bindPanel(panel) {
    qs('[data-nai-close]', panel).addEventListener('click', () => togglePanel(false));
    qs('[data-nai-preview]', panel).addEventListener('click', refreshPreview);
    qsa('[data-nai-run]', panel).forEach((button) => button.addEventListener('click', runImport));
    qs('[data-nai-add-keys]', panel).addEventListener('click', addKeysToPool);
    qs('[data-nai-copy-payload]', panel).addEventListener('click', copyFirstPayload);
    qs('[data-nai-refresh-job]', panel).addEventListener('click', refreshActiveJobStatus);
    qs('[data-nai-toggle-job]', panel).addEventListener('click', toggleActiveJobRunning);
    qs('[data-nai-export-job]', panel).addEventListener('click', exportActiveJob);
    qs('[data-nai-load-template]', panel).addEventListener('click', loadSelectedTemplate);
    qs('[data-nai-refresh-templates]', panel).addEventListener('click', loadTemplates);
    qs('[data-nai-refresh-site]', panel).addEventListener('click', () => {
      updateSiteInfo();
      appendLog('已刷新站点信息。');
    });
    qs('[data-nai-refresh-groups]', panel).addEventListener('click', loadGroups);
    qs('[data-nai-refresh-base-url]', panel).addEventListener('click', () => {
      updateBaseUrlDisplay();
      appendLog('已刷新内置 API 地址显示。');
    });
    qs('[data-nai-group-select]', panel).addEventListener('change', applySelectedGroup);
    qs('[data-nai-group-trigger]', panel).addEventListener('click', () => {
      setGroupPickerOpen(qs('[data-nai-group-trigger]', panel).getAttribute('aria-expanded') !== 'true');
    });
    qs('[data-nai-group-menu]', panel).addEventListener('click', (event) => {
      if (!(event.target instanceof Element)) return;
      const option = event.target.closest('[data-nai-group-option]');
      if (!option) return;
      toggleGroupOption(option.getAttribute('data-nai-group-option') || '');
    });
    qs('[data-nai-open-params]', panel).addEventListener('click', () => setParamsPaneOpen(true));
    qs('[data-nai-apply-strategy]', panel).addEventListener('click', applyRuntimeJobConfig);
    qs('[data-nai-export-work]', panel).addEventListener('click', exportWorkspace);
    qs('[data-nai-import-work]', panel).addEventListener('click', () => qs('[data-nai-import-work-file]', panel)?.click());
    qs('[data-nai-import-work-file]', panel).addEventListener('change', importWorkspaceFromFile);
    qs('[data-nai-reset-work]', panel).addEventListener('click', resetWorkspace);
    qs('[data-nai-toggle-params]', panel)?.addEventListener('click', () => {
      const isOpen = panel.getAttribute('data-nai-right-open') !== 'false';
      setParamsPaneOpen(!isOpen);
    });

    panel.addEventListener('click', (event) => {
      if (!(event.target instanceof Element)) return;
      const keyTab = event.target.closest('[data-nai-key-tab]');
      if (keyTab) {
        setKeyTab(keyTab.getAttribute('data-nai-key-tab') || 'list');
        return;
      }
      const jobTab = event.target.closest('[data-nai-job-tab]');
      if (jobTab) {
        setJobTab(jobTab.getAttribute('data-nai-job-tab') || 'stats');
        return;
      }
      if (event.target.closest('[data-nai-name-add-segment]')) {
        const config = collectConfig(false);
        config.nameSegments = normalizeNameSegments(config.nameSegments, config);
        if (config.nameSegments.length < MAX_NAME_SEGMENTS) config.nameSegments.push('');
        saveConfig(config);
        renderNameEditor(config);
        refreshPreview();
        return;
      }
      const remove = event.target.closest('[data-nai-name-remove]');
      if (remove) {
        const config = collectConfig(false);
        const index = Number.parseInt(remove.getAttribute('data-nai-name-remove') || '-1', 10);
        config.nameSegments = normalizeNameSegments(config.nameSegments, config);
        if (index >= 0 && config.nameSegments.length > 1) config.nameSegments.splice(index, 1);
        saveConfig(config);
        renderNameEditor(config);
        refreshPreview();
      }
    });

    panel.addEventListener('change', (event) => {
      if (!(event.target instanceof Element)) return;
      if (!event.target.matches('[data-nai-name-segment-type]')) return;
      const config = collectConfig(false);
      saveConfig(config);
      renderNameEditor(config);
      refreshPreview();
    });

    panel.addEventListener('input', (event) => {
      if (!(event.target instanceof Element)) return;
      if (!event.target.matches('[data-nai-name-setting]')) return;
      saveConfig(collectConfig(false));
      refreshPreview();
    });

    qsa('[data-nai-field], [data-nai-check]', panel).forEach((el) => {
      el.addEventListener('input', () => {
        saveConfig(collectConfig(false));
        if (el.getAttribute('data-nai-field') === 'group') {
          updateGroupSelectFromInput();
          updateJobPreview();
        }
        if (isStrategyField(el)) markStrategyDirty();
        refreshPreview();
        updateJobPreview();
      });
      el.addEventListener('change', () => {
        saveConfig(collectConfig(false));
        if (isStrategyField(el)) markStrategyDirty();
        if (el.getAttribute('data-nai-field') === 'typePreset') {
          updateTypePicker();
          updateBaseUrlDisplay();
          loadTemplates();
        }
        if (el.getAttribute('data-nai-field') === 'group') {
          updateGroupSelectFromInput();
          updateJobPreview();
        }
        refreshPreview();
        updateJobPreview();
      });
    });

    qsa('[data-nai-runtime-field], [data-nai-runtime-check]', panel).forEach((el) => {
      el.addEventListener('input', () => {
        if (isStrategyField(el)) markStrategyDirty();
      });
      el.addEventListener('change', () => {
        if (isStrategyField(el)) markStrategyDirty();
      });
    });

    qs('#nai-keys', panel).addEventListener('input', () => {
      refreshPreview();
    });

    panel.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') setGroupPickerOpen(false);
    });

    document.addEventListener('click', (event) => {
      const picker = qs('[data-nai-group-picker]', panel);
      if (picker?.contains(event.target)) return;
      setGroupPickerOpen(false);
    });
  }

  function setParamsPaneOpen(open) {
    const panel = document.getElementById(SCRIPT_ID);
    if (!panel) return;
    panel.setAttribute('data-nai-right-open', String(open));
    const toggle = qs('[data-nai-toggle-params]', panel);
    if (toggle) toggle.textContent = open ? '收起' : '展开';
    updateJobPreview();
  }

  function setKeyTab(tab) {
    const panel = document.getElementById(SCRIPT_ID);
    if (!panel) return;
    qsa('[data-nai-key-tab]', panel).forEach((button) => {
      button.setAttribute('data-active', String(button.getAttribute('data-nai-key-tab') === tab));
    });
    const listPanel = qs('#nai-keyListPanel', panel);
    const statsPanel = qs('#nai-keyStatsPanel', panel);
    if (listPanel) listPanel.hidden = tab !== 'list';
    if (statsPanel) statsPanel.hidden = tab !== 'stats';
  }

  function setJobTab(tab) {
    const panel = document.getElementById(SCRIPT_ID);
    if (!panel) return;
    qsa('[data-nai-job-tab]', panel).forEach((button) => {
      button.setAttribute('data-active', String(button.getAttribute('data-nai-job-tab') === tab));
    });
    const statsPanel = qs('#nai-jobStatsPanel', panel);
    const batchesPanel = qs('#nai-jobBatchesPanel', panel);
    const logsPanel = qs('#nai-jobLogsPanel', panel);
    if (statsPanel) statsPanel.hidden = tab !== 'stats';
    if (batchesPanel) batchesPanel.hidden = tab !== 'batches';
    if (logsPanel) logsPanel.hidden = tab !== 'logs';
  }

  function isStrategyField(el) {
    const field = el?.getAttribute?.('data-nai-runtime-field');
    const check = el?.getAttribute?.('data-nai-runtime-check');
    return ['targetAliveSize', 'aliveThreshold', 'replenishBatchSize', 'monitorIntervalSec'].includes(field) || check === 'autoRefill';
  }

  function markStrategyDirty() {
    if (!state.activeJob) return;
    state.strategyDirty = true;
    updateJobControls();
  }

  function collectRuntimeConfig(config) {
    return {
      autoRefill: config.autoRefill === true,
      targetAliveSize: parsePositiveInt(config.targetAliveSize, 10, 0),
      aliveThreshold: parsePositiveInt(config.aliveThreshold, 5, 0),
      replenishBatchSize: parsePositiveInt(config.replenishBatchSize, 10, 1),
      monitorIntervalSec: parsePositiveInt(config.monitorIntervalSec, 60, 5),
      updatedAt: nowIso(),
    };
  }

  function collectRuntimeEditorConfig() {
    const panel = document.getElementById(SCRIPT_ID);
    const config = {
      autoRefill: qs('[data-nai-runtime-check="autoRefill"]', panel)?.checked === true,
      targetAliveSize: qs('[data-nai-runtime-field="targetAliveSize"]', panel)?.value || DEFAULT_CONFIG.targetAliveSize,
      aliveThreshold: qs('[data-nai-runtime-field="aliveThreshold"]', panel)?.value || DEFAULT_CONFIG.aliveThreshold,
      replenishBatchSize: qs('[data-nai-runtime-field="replenishBatchSize"]', panel)?.value || DEFAULT_CONFIG.replenishBatchSize,
      monitorIntervalSec: qs('[data-nai-runtime-field="monitorIntervalSec"]', panel)?.value || DEFAULT_CONFIG.monitorIntervalSec,
    };
    return collectRuntimeConfig(config);
  }

  function runtimeConfigSummary(runtimeConfig) {
    if (!runtimeConfig) return '-';
    return `${runtimeConfig.autoRefill === false ? '手动监控' : '自动补货'} / 保活 ${runtimeConfig.targetAliveSize} / 低于 ${runtimeConfig.aliveThreshold} / 添加 ${runtimeConfig.replenishBatchSize} / 间隔 ${runtimeConfig.monitorIntervalSec} 秒`;
  }

  function configForJob(job) {
    const snapshot = job?.configSnapshot || collectConfig(false);
    const runtime = job?.runtimeConfig || collectRuntimeConfig(snapshot);
    return {
      ...snapshot,
      autoRefill: runtime.autoRefill !== false,
      targetAliveSize: String(runtime.targetAliveSize),
      aliveThreshold: String(runtime.aliveThreshold),
      replenishBatchSize: String(runtime.replenishBatchSize),
      monitorIntervalSec: String(runtime.monitorIntervalSec),
    };
  }

  function applyRuntimeJobConfig() {
    const job = state.activeJob;
    if (!job || job.stopped) return;
    const nextConfig = collectRuntimeEditorConfig();
    const previous = runtimeConfigSummary(job.runtimeConfig);
    const next = runtimeConfigSummary(nextConfig);
    const ok = window.confirm(`确认将当前作业策略更新为：${next}？`);
    if (!ok) return;
    job.runtimeConfig = nextConfig;
    state.strategyDirty = false;
    recordJobLog(job, `运行策略已更新：${previous} -> ${next}。`);
    if (!job.paused && !job.stopped) startMonitorLoop();
    persistWorkspaceState();
    updateJobStats();
    updateJobControls();
  }

  function setTypePickerOpen(open) {
    const trigger = qs('[data-nai-type-trigger]');
    const menu = qs('[data-nai-type-menu]');
    if (!trigger || !menu) return;
    trigger.setAttribute('aria-expanded', String(open));
    menu.hidden = !open;
  }

  function updateTypePicker() {
    const select = qs('#nai-typePreset');
    const trigger = qs('[data-nai-type-trigger]');
    const menu = qs('[data-nai-type-menu]');
    if (!select || !trigger || !menu) return;
    trigger.innerHTML = renderTypePickerValue(select.value);
    menu.innerHTML = renderTypeMenuOptions(select.value);
  }

  function chooseTypePreset(value) {
    const select = qs('#nai-typePreset');
    if (!select) return;
    select.value = String(value);
    updateTypePicker();
    setTypePickerOpen(false);
    select.dispatchEvent(new Event('change', { bubbles: true }));
  }

  function setupTypePicker(panel) {
    const trigger = qs('[data-nai-type-trigger]', panel);
    const menu = qs('[data-nai-type-menu]', panel);
    if (!trigger || !menu) return;

    trigger.addEventListener('click', () => {
      setTypePickerOpen(trigger.getAttribute('aria-expanded') !== 'true');
    });

    menu.addEventListener('click', (event) => {
      if (!(event.target instanceof Element)) return;
      const option = event.target.closest('[data-nai-type-option]');
      if (!option) return;
      chooseTypePreset(option.getAttribute('data-nai-type-option'));
    });

    panel.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') setTypePickerOpen(false);
    });

    document.addEventListener('click', (event) => {
      const picker = qs('[data-nai-type-picker]', panel);
      if (picker?.contains(event.target)) return;
      setTypePickerOpen(false);
    });
  }

  function togglePanel(open) {
    state.open = open;
    const panel = document.getElementById(SCRIPT_ID);
    if (panel) panel.setAttribute('data-open', String(open));
  }

  function collectConfig(includeKeys = true) {
    const panel = document.getElementById(SCRIPT_ID);
    const config = { ...DEFAULT_CONFIG };
    for (const id of fieldIds) {
      const el = qs(`[data-nai-field="${id}"]`, panel);
      if (el) config[id] = el.value;
    }
    for (const id of checkboxIds) {
      const el = qs(`[data-nai-check="${id}"]`, panel);
      if (el) config[id] = el.checked;
    }
    if (includeKeys) {
      config.keys = qs('#nai-keys', panel)?.value || '';
    }
    collectNameConfig(panel, config);
    return config;
  }

  function collectNameConfig(panel, config) {
    const saved = loadConfig();
    const segments = qsa('[data-nai-name-segment-type]', panel).map((el) => normalizeSegmentType(el.value));
    config.nameSegments = segments.length ? segments.slice(0, MAX_NAME_SEGMENTS) : normalizeNameSegments(saved.nameSegments, saved);

    const settings = normalizeNameSegmentSettings(saved.nameSegmentSettings, saved);
    qsa('[data-nai-name-setting]', panel).forEach((el) => {
      const path = String(el.getAttribute('data-nai-name-setting') || '').split('.');
      if (path.length !== 2) return;
      const [slot, key] = path;
      if (!settings[slot]) settings[slot] = defaultSegmentSettings(slot);
      settings[slot][key] = el.value;
    });
    config.nameSegmentSettings = settings;
  }

  function renderNameEditor(config) {
    const builderHost = qs('#nai-nameBuilderHost');
    const settingsHost = qs('#nai-nameSettingsHost');
    if (builderHost) builderHost.innerHTML = renderNameBuilderHtml(config);
    if (settingsHost) settingsHost.innerHTML = renderNameSegmentSettingsHtml(config);
  }

  function getChannelType(config) {
    const value = config.typePreset || DEFAULT_CONFIG.typePreset;
    const type = Number.parseInt(value, 10);
    if (!Number.isInteger(type) || type < 0) {
      throw new Error('类型编号无效');
    }
    return type;
  }

  function stripKeyCandidate(value) {
    return String(value || '')
      .trim()
      .replace(/^```[a-zA-Z0-9_-]*\s*/, '')
      .replace(/```$/, '')
      .replace(/^\s*(?:[-*]|\d+[.)])\s+/, '')
      .replace(/^\s*(?:api[-_\s]*)?key\s*[:=]\s*/i, '')
      .replace(/^\s*(?:token|secret|authorization)\s*[:=]\s*/i, '')
      .replace(/^Bearer\s+/i, '')
      .replace(/^["'`]+|["'`,;]+$/g, '')
      .trim();
  }

  function looksLikePlainKey(value) {
    const text = stripKeyCandidate(value);
    if (text.length < 16) return false;
    if (/^(?:api\s*)?key$|^name$|^model$|^models$/i.test(text)) return false;
    if (/^https?:\/\//i.test(text)) return false;
    if (/[,，;]/.test(text)) return false;
    return !/\s/.test(text);
  }

  function looksLikeLoosePrefixedKey(value) {
    const text = stripKeyCandidate(value);
    if (text.length < 8) return false;
    if (/^https?:\/\//i.test(text)) return false;
    if (/[,，;\s]/.test(text)) return false;
    return /^(?:sk-|sk-ant-|xai-|gsk_|hf_|AIza|ya29\.)/i.test(text);
  }

  function extractKnownKeyTokens(value) {
    const text = String(value || '');
    const patterns = [
      /sk-ant-[A-Za-z0-9._-]{12,}/g,
      /sk-[A-Za-z0-9._-]{12,}/g,
      /AIza[0-9A-Za-z_-]{20,}/g,
      /xai-[A-Za-z0-9._-]{12,}/g,
      /gsk_[A-Za-z0-9._-]{12,}/g,
      /hf_[A-Za-z0-9._-]{12,}/g,
      /ya29\.[A-Za-z0-9._-]{12,}/g,
      /[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}/g,
    ];
    return patterns.flatMap((pattern) => text.match(pattern) || []).map(stripKeyCandidate);
  }

  function collectJsonKeys(value, keys) {
    if (typeof value === 'string') {
      const candidate = stripKeyCandidate(value);
      if (looksLikePlainKey(candidate)) keys.push(candidate);
      return;
    }
    if (Array.isArray(value)) {
      value.forEach((item) => collectJsonKeys(item, keys));
      return;
    }
    if (!value || typeof value !== 'object') return;

    if (
      typeof value.private_key === 'string' &&
      typeof value.client_email === 'string'
    ) {
      keys.push(JSON.stringify(value));
      return;
    }
    if (
      typeof value.access_token === 'string' &&
      typeof value.refresh_token === 'string' &&
      (typeof value.account_id === 'string' || typeof value.account_id === 'number')
    ) {
      keys.push(JSON.stringify(value));
      return;
    }

    for (const [key, item] of Object.entries(value)) {
      if (/(api[-_ ]?key|key|token|secret|credential)/i.test(key)) {
        collectJsonKeys(item, keys);
      }
    }
  }

  function addKeyCandidate(keys, raw, allowFallback = true, allowLoosePrefixed = false) {
    const knownTokens = extractKnownKeyTokens(raw);
    if (knownTokens.length > 0) {
      keys.push(...knownTokens);
      return;
    }

    const candidate = stripKeyCandidate(raw);
    if (!candidate) return;
    const keyValueMatch = candidate.match(/^(?:[A-Za-z0-9_. -]+)?(?:api[-_ ]?key|key|token|secret|credential)\s*[:=]\s*(.+)$/i);
    const normalized = keyValueMatch ? stripKeyCandidate(keyValueMatch[1]) : candidate;
    if (allowFallback && looksLikePlainKey(normalized)) keys.push(normalized);
    if (allowLoosePrefixed && looksLikeLoosePrefixedKey(normalized)) keys.push(normalized);
  }

  function parseKeys(raw, dedupe, options = {}) {
    const text = String(raw || '').trim();
    const keys = [];
    if (!text) return keys;
    const allowLoosePrefixed = Boolean(options.allowLoosePrefixed);

    try {
      const parsed = JSON.parse(text);
      if (
        parsed &&
        typeof parsed === 'object' &&
        !Array.isArray(parsed) &&
        typeof parsed.private_key === 'string' &&
        typeof parsed.client_email === 'string'
      ) {
        keys.push(text);
      } else {
        collectJsonKeys(parsed, keys);
      }
    } catch {
      /* continue with text extraction */
    }

    const normalized = text
      .replace(/\r/g, '\n')
      .replace(/[，；;]/g, '\n')
      .replace(/```[a-zA-Z0-9_-]*\n/g, '')
      .replace(/```/g, '');

    normalized
      .split(/\n+/)
      .map((item) => item.trim())
      .filter(Boolean)
      .forEach((line) => {
        if (/^(?:#|\/\/)/.test(line)) return;
        addKeyCandidate(keys, line, true, allowLoosePrefixed);
        line
          .split(/[,，\t]+/)
          .map((item) => item.trim())
          .filter(Boolean)
          .forEach((part) => addKeyCandidate(keys, part, true, allowLoosePrefixed));
      });

    if (!dedupe) return keys;
    return Array.from(new Set(keys));
  }

  function normalizeList(raw) {
    return Array.from(
      new Set(
        String(raw || '')
          .split(/[,\n]/)
          .map((item) => item.trim())
          .filter(Boolean)
      )
    ).join(',');
  }

  function parseOptionalJsonObject(raw, label) {
    const trimmed = String(raw || '').trim();
    if (!trimmed) return null;
    let parsed;
    try {
      parsed = JSON.parse(trimmed);
    } catch (error) {
      throw new Error(`${label} 不是有效 JSON: ${error.message}`);
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      throw new Error(`${label} 必须是 JSON 对象`);
    }
    return parsed;
  }

  function normalizeOptionalJsonString(raw, label) {
    const parsed = parseOptionalJsonObject(raw, label);
    return parsed ? JSON.stringify(parsed) : null;
  }

  function normalizeModelMapping(raw) {
    const parsed = parseOptionalJsonObject(raw, '模型映射');
    if (!parsed) return null;
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value !== 'string') {
        throw new Error(`模型映射 ${key} 的值必须是字符串`);
      }
    }
    return JSON.stringify(parsed);
  }

  function numberToken(settings, index) {
    const start = Number.parseInt(settings.numberStart || '1', 10);
    const pad = Math.max(0, Number.parseInt(settings.numberPad || '0', 10) || 0);
    const value = (Number.isFinite(start) ? start : 1) + index;
    return String(value).padStart(pad, '0');
  }

  function alphaToIndex(value) {
    const text = String(value || 'A').trim();
    const first = text[0] || 'A';
    const code = first.toUpperCase().charCodeAt(0);
    if (code < 65 || code > 90) return 0;
    return code - 65;
  }

  function indexToAlpha(index, uppercase) {
    let value = Math.max(0, index);
    let output = '';
    do {
      output = String.fromCharCode(65 + (value % 26)) + output;
      value = Math.floor(value / 26) - 1;
    } while (value >= 0);
    return uppercase ? output : output.toLowerCase();
  }

  function randomCode(length = 6) {
    const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    const bytes = new Uint8Array(length);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (byte) => alphabet[byte % alphabet.length]).join('');
  }

  function formatDateByPattern(date, pattern) {
    const pad = (value) => String(value).padStart(2, '0');
    const milli = String(date.getMilliseconds()).padStart(3, '0');
    const tokens = {
      yyyy: String(date.getFullYear()),
      YYYY: String(date.getFullYear()),
      MM: pad(date.getMonth() + 1),
      dd: pad(date.getDate()),
      DD: pad(date.getDate()),
      HH: pad(date.getHours()),
      mm: pad(date.getMinutes()),
      ss: pad(date.getSeconds()),
      SSS: milli,
    };
    return String(pattern || 'yyyyMMdd-HHmmss').replace(/yyyy|YYYY|SSS|MM|dd|DD|HH|mm|ss/g, (token) => tokens[token] ?? token);
  }

  function timestampToken(date = new Date(), format = 'yyyyMMdd-HHmmss') {
    return formatDateByPattern(date, format);
  }

  function dateToken(date = new Date(), format = 'yyyyMMdd') {
    return formatDateByPattern(date, format);
  }

  function keyPreview(key) {
    if (!key) return '';
    if (key.length <= 12) return `${key.slice(0, 4)}...`;
    return `${key.slice(0, 8)}...${key.slice(-4)}`;
  }

  function ensureNameSeed(config, keys) {
    const seedKey = JSON.stringify({
      keys,
      nameSegments: normalizeNameSegments(config.nameSegments, config),
      nameSegmentSettings: normalizeNameSegmentSettings(config.nameSegmentSettings, config),
    });
    if (state.nameSeedKey === seedKey) return;
    state.nameSeedKey = seedKey;
    state.nameTimestamp = new Date().toISOString();
    state.nameDate = state.nameTimestamp;
    state.randomCodes = new Map();
  }

  function stableRandomCode(key, index, slot = '') {
    const cacheKey = `${slot}:${index}:${key}`;
    if (!state.randomCodes.has(cacheKey)) {
      state.randomCodes.set(cacheKey, randomCode(6));
    }
    return state.randomCodes.get(cacheKey);
  }

  function makeName(config, key, index) {
    const segments = normalizeNameSegments(config.nameSegments, config);
    const settings = normalizeNameSegmentSettings(config.nameSegmentSettings, config);
    const seedDate = new Date(state.nameTimestamp || Date.now());
    return segments.map((type, segmentIndex) => {
      const slot = slotLabel(segmentIndex);
      const setting = settings[slot] || defaultSegmentSettings(slot);
      if (type === 'text') return String(setting.text || '');
      if (type === 'num') return numberToken(setting, index);
      if (type === 'alpha') {
        const alphaStart = String(setting.alphaStart || 'A');
        const uppercase = alphaStart[0] !== alphaStart[0]?.toLowerCase();
        return indexToAlpha(alphaToIndex(alphaStart) + index, uppercase);
      }
      if (type === 'rand6') return stableRandomCode(key, index, slot);
      if (type === 'ts') return timestampToken(seedDate, setting.tsFormat || 'yyyyMMdd-HHmmss');
      if (type === 'date') return dateToken(seedDate, setting.dateFormat || 'yyyyMMdd');
      if (type === 'key8') return String(key || '').slice(0, 8);
      return '';
    }).join('');
  }

  function buildRowsForKeys(config, keys, startIndex = 0) {
    ensureNameSeed(config, keys);
    return keys.map((key, index) => ({
      index: startIndex + index,
      key,
      name: makeName(config, key, startIndex + index),
    }));
  }

  function buildRows(config) {
    const keys = parseKeys(config.keys, config.dedupeKeys);
    return buildRowsForKeys(config, keys, 0);
  }

  function validateJobConfig(config, rows = []) {
    getChannelType(config);
    if (rows.length && rows.some((row) => !String(row.name || '').trim())) throw new Error('名称组合不能为空');
    if (!normalizeList(config.models)) throw new Error('模型不能为空');
    if (!String(config.group || '').trim()) throw new Error('分组不能为空');
    normalizeModelMapping(config.modelMapping);
    normalizeOptionalJsonString(config.settingJson, 'setting JSON');
    normalizeOptionalJsonString(config.settingsJson, 'settings JSON');
    normalizeOptionalJsonString(config.paramOverride, 'param_override JSON');
    normalizeOptionalJsonString(config.headerOverride, 'header_override JSON');
    normalizeOptionalJsonString(config.statusCodeMapping, 'status_code_mapping JSON');
  }

  function validateConfig(config, rows) {
    validateJobConfig(config, rows);
    if (!rows.length) throw new Error('请先粘贴至少一个 API key');
  }

  function refreshPreview() {
    const preview = qs('#nai-preview');
    if (!preview) return;
    let rows = [];
    let error = '';
    try {
      const config = collectConfig(true);
      rows = buildRows(config);
      validateConfig({ ...config, keys: rows.map((row) => row.key).join('\n') }, rows);
    } catch (err) {
      error = err.message;
    }

    if (!rows.length) {
      preview.innerHTML = `<div class="nai-bulk-help" style="padding: 10px;">粘贴 key 后显示预览。</div>`;
      return;
    }

    const visibleRows = rows.slice(0, 50);
    const body = visibleRows
      .map(
        (row) => `
          <tr>
            <td>${row.index + 1}</td>
            <td>${escapeHtml(row.name)}</td>
            <td>${escapeHtml(keyPreview(row.key))}</td>
          </tr>
        `
      )
      .join('');

    const suffix =
      rows.length > visibleRows.length
        ? `<div class="nai-bulk-help" style="padding: 8px 9px;">只显示前 ${visibleRows.length} 条，共 ${rows.length} 条。</div>`
        : '';
    const errorHtml = error
      ? `<div class="nai-bulk-error" style="padding: 8px 9px;">${escapeHtml(error)}</div>`
      : `<div class="nai-bulk-ok" style="padding: 8px 9px;">预览 ${rows.length} 条，可提交。</div>`;

    preview.innerHTML = `
      ${errorHtml}
      <table>
        <thead><tr><th>#</th><th>渠道名</th><th>key</th></tr></thead>
        <tbody>${body}</tbody>
      </table>
      ${suffix}
    `;
  }

  function buildSettingJson(config) {
    const setting = parseOptionalJsonObject(config.settingJson, 'setting JSON') || {};
    return JSON.stringify({
      force_format: setting.force_format === true,
      thinking_to_content: setting.thinking_to_content === true,
      proxy: typeof setting.proxy === 'string' ? setting.proxy : '',
      pass_through_body_enabled: setting.pass_through_body_enabled === true,
      system_prompt: typeof setting.system_prompt === 'string' ? setting.system_prompt : '',
      system_prompt_override: setting.system_prompt_override === true,
    });
  }

  function buildOtherSettingsJson(config, type) {
    const settings = parseOptionalJsonObject(config.settingsJson, 'settings JSON') || {};

    if (type === 1 || type === 14) {
      settings.allow_service_tier = config.allowServiceTier === true;
    } else {
      delete settings.allow_service_tier;
    }

    if (type === 14) {
      settings.allow_inference_geo = config.allowInferenceGeo === true;
      settings.allow_speed = config.allowSpeed === true;
      settings.claude_beta_query = config.claudeBetaQuery === true;
    } else {
      delete settings.allow_speed;
      delete settings.claude_beta_query;
      if (type !== 1) delete settings.allow_inference_geo;
    }

    if (!('disable_task_polling_sleep' in settings)) {
      settings.disable_task_polling_sleep = false;
    }

    return JSON.stringify(settings);
  }

  function numberOrNull(value) {
    if (value === '' || value === null || value === undefined) return null;
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }

  function buildPayload(row, config) {
    const type = getChannelType(config);
    const modelMapping = normalizeModelMapping(config.modelMapping);
    const payload = {
      mode: 'single',
      channel: {
        name: row.name,
        type,
        base_url: null,
        key: row.key,
        openai_organization: null,
        models: normalizeList(config.models),
        group: normalizeList(config.group),
        model_mapping: modelMapping,
        priority: numberOrNull(config.priority),
        weight: numberOrNull(config.weight),
        test_model: null,
        auto_ban: config.autoBan ? 1 : 0,
        status: config.status ? 1 : 2,
        status_code_mapping: normalizeOptionalJsonString(config.statusCodeMapping, 'status_code_mapping JSON'),
        tag: String(config.tag || '').trim() || null,
        remark: String(config.remark || ''),
        setting: buildSettingJson(config),
        param_override: normalizeOptionalJsonString(config.paramOverride, 'param_override JSON'),
        header_override: normalizeOptionalJsonString(config.headerOverride, 'header_override JSON'),
        settings: buildOtherSettingsJson(config, type),
        other: String(config.other || ''),
      },
    };
    return payload;
  }

  function getApiRoot() {
    return API_ROOT;
  }

  function apiUrl(_config, suffix = '') {
    const root = getApiRoot();
    return `${root}${suffix}`;
  }

  function normalizeUserId(value) {
    const text = String(value ?? '').trim();
    if (!text || text === 'null' || text === 'undefined') return '';
    return text;
  }

  function getUserId() {
    try {
      const uid = normalizeUserId(localStorage.getItem('uid'));
      if (uid) return uid;
    } catch {
      /* fall through to old UI storage */
    }

    try {
      const rawUser = localStorage.getItem('user');
      if (!rawUser) return '';
      const user = JSON.parse(rawUser);
      return (
        normalizeUserId(user?.id) ||
        normalizeUserId(user?.user?.id) ||
        normalizeUserId(user?.data?.id)
      );
    } catch {
      return '';
    }
  }

  async function apiRequest(url, options = {}) {
    const uid = getUserId();
    const headers = {
      Accept: 'application/json',
      'Cache-Control': 'no-store',
      ...options.headers,
    };
    if (options.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }
    if (uid) {
      headers['New-Api-User'] = uid;
    }

    const response = await fetch(url, {
      credentials: 'include',
      ...options,
      headers,
    });
    const text = await response.text();
    let data = null;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      data = { success: false, message: text || response.statusText };
    }
    if (!response.ok) {
      const message = data?.message || `${response.status} ${response.statusText}`;
      throw new Error(message);
    }
    return data;
  }

  function normalizeChannelResult(result) {
    const data = result?.data ?? result;
    if (!data || typeof data !== 'object') return null;
    return data.channel || data.item || data;
  }

  function channelsFromListResult(result) {
    const data = result?.data ?? result;
    const lists = [
      data?.items,
      data?.channels,
      data?.list,
      data?.rows,
      result?.items,
      result?.channels,
      Array.isArray(data) ? data : null,
    ];

    for (const list of lists) {
      if (Array.isArray(list)) return list.filter(Boolean);
    }
    return [];
  }

  function nowIso() {
    return new Date().toISOString();
  }

  function parsePositiveInt(value, fallback, min = 0) {
    const parsed = Number.parseInt(value, 10);
    if (!Number.isFinite(parsed)) return fallback;
    return Math.max(min, parsed);
  }

  function sameLocalDate(iso, date = new Date()) {
    if (!iso) return false;
    const value = new Date(iso);
    return value.getFullYear() === date.getFullYear() &&
      value.getMonth() === date.getMonth() &&
      value.getDate() === date.getDate();
  }

  function formatLocalDateTime(iso) {
    if (!iso) return '-';
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '-';
    return date.toLocaleString();
  }

  function defaultJobName(config, date = new Date()) {
    const [, typeLabel] = channelTypeEntry(config.typePreset || DEFAULT_CONFIG.typePreset);
    return `${formatDateByPattern(date, 'yyyyMMdd-HHmmss')}-${typeLabel}`;
  }

  function resolveJobName(config) {
    return String(config.jobName || '').trim() || defaultJobName(config);
  }

  function formatDuration(ms) {
    if (!Number.isFinite(ms) || ms <= 0) return '-';
    const totalSeconds = Math.floor(ms / 1000);
    const days = Math.floor(totalSeconds / 86400);
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;
    if (days > 0) return `${days}天 ${hours}小时`;
    if (hours > 0) return `${hours}小时 ${minutes}分`;
    if (minutes > 0) return `${minutes}分 ${seconds}秒`;
    return `${seconds}秒`;
  }

  function channelTimeToIso(value, fallback = null) {
    const numeric = Number(value);
    if (!Number.isFinite(numeric) || numeric <= 0) return fallback;
    const ms = numeric > 100000000000 ? numeric : numeric * 1000;
    return new Date(ms).toISOString();
  }

  function statusLabel(status) {
    if (Number(status) === 1) return '启用';
    if (Number(status) === 2) return '禁用';
    if (Number(status) === 3) return '自动禁用';
    return status === null || status === undefined ? '未知' : String(status);
  }

  function numericQuota(channel) {
    const value = Number(channel?.used_quota ?? channel?.usedQuota ?? channel?.UsedQuota ?? 0);
    return Number.isFinite(value) ? value : 0;
  }

  function ensureKeyPoolEntry(key) {
    if (state.keyPoolSet.has(key)) return null;
    const entry = {
      key,
      keyPreview: keyPreview(key),
      addedAt: nowIso(),
      attemptedAt: null,
      usedAt: null,
      channelCreatedAt: null,
      channelId: null,
      channelName: '',
      status: null,
      statusText: '未使用',
      disabledAt: null,
      lastSeenAt: null,
      usedQuota: 0,
      batchNo: null,
      error: '',
    };
    state.keyPoolSet.add(key);
    state.keyPool.push(entry);
    return entry;
  }

  function syncKeyPoolFromInput(config, source = '', options = {}) {
    const keys = parseKeys(config.keys || qs('#nai-keys')?.value || '', config.dedupeKeys, options);
    let added = 0;
    for (const key of keys) {
      if (ensureKeyPoolEntry(key)) added += 1;
    }
    if (added > 0 && state.activeJob && source) {
      recordJobLog(state.activeJob, `key 库新增 ${added} 个 key（来源：${source}）。`);
    }
    return added;
  }

  function addKeysToPool() {
    const config = collectConfig(true);
    const added = syncKeyPoolFromInput(config, state.activeJob ? '手动入库' : '', {
      allowLoosePrefixed: true,
    });
    if (added > 0) {
      appendLog(`已添加 ${added} 个 key 到 key 库。`);
    } else {
      appendLog('没有发现新的有效 key，或这些 key 已经在库中。');
    }
    refreshPreview();
    updateJobStats();
    persistWorkspaceState();
    if (added > 0 && state.activeJob && !state.activeJob.stopped && !state.activeJob.paused) {
      void monitorActiveJob();
    }
  }

  function createJob(config) {
    const snapshot = { ...config };
    delete snapshot.keys;
    const name = resolveJobName(config);
    snapshot.jobName = name;
    return {
      id: `job-${Date.now()}`,
      name,
      site: currentSiteInfo(),
      startedAt: nowIso(),
      stoppedAt: null,
      stopped: false,
      paused: false,
      runtimeConfig: collectRuntimeConfig(config),
      nextIndex: 0,
      configSnapshot: snapshot,
      keys: state.keyPool,
      batches: [],
      logs: [],
      noKeyLogged: false,
    };
  }

  function recordJobLog(job, message, kind = '') {
    if (!job) return;
    const entry = {
      at: nowIso(),
      kind: kind || 'info',
      message,
    };
    job.logs.push(entry);
    appendLog(`[作业] ${message}`, kind === 'error' ? 'error' : '');
    persistWorkspaceState();
  }

  function usedEntries(job) {
    return (job?.keys || state.keyPool).filter((entry) => entry.attemptedAt);
  }

  function createdEntries(job) {
    return (job?.keys || state.keyPool).filter((entry) => entry.channelId || entry.channelCreatedAt);
  }

  function availableEntries(job) {
    return (job?.keys || state.keyPool).filter((entry) => !entry.attemptedAt);
  }

  function calculateJobStats(job = state.activeJob) {
    const entries = job?.keys || state.keyPool;
    const attempted = entries.filter((entry) => entry.attemptedAt);
    const created = entries.filter((entry) => entry.channelId || entry.channelCreatedAt);
    const alive = created.filter((entry) => Number(entry.status) === 1);
    const disabled = created.filter((entry) => entry.status !== null && Number(entry.status) !== 1);
    const unknown = created.filter((entry) => entry.status === null);
    const quotas = created.map((entry) => Number(entry.usedQuota || 0)).filter(Number.isFinite);
    const now = Date.now();
    const lifetimes = created
      .map((entry) => {
        const start = Date.parse(entry.channelCreatedAt || entry.usedAt || entry.attemptedAt || '');
        if (!Number.isFinite(start)) return null;
        const end = Date.parse(entry.disabledAt || '') || now;
        return Math.max(0, end - start);
      })
      .filter((value) => value !== null);
    const firstCreated = created
      .map((entry) => entry.channelCreatedAt)
      .filter(Boolean)
      .sort()[0] || '';

    return {
      totalKeys: entries.length,
      unusedKeys: entries.filter((entry) => !entry.attemptedAt).length,
      attempted: attempted.length,
      created: created.length,
      alive: alive.length,
      disabled: disabled.length,
      unknown: unknown.length,
      todayCreated: created.filter((entry) => sameLocalDate(entry.channelCreatedAt)).length,
      batches: job?.batches?.length || 0,
      firstCreated,
      jobDurationMs: job ? now - Date.parse(job.startedAt) : 0,
      averageLifetimeMs: lifetimes.length ? lifetimes.reduce((sum, value) => sum + value, 0) / lifetimes.length : 0,
      totalQuota: quotas.reduce((sum, value) => sum + value, 0),
      averageQuota: quotas.length ? quotas.reduce((sum, value) => sum + value, 0) / quotas.length : 0,
      maxQuota: quotas.length ? Math.max(...quotas) : 0,
      minQuota: quotas.length ? Math.min(...quotas) : 0,
    };
  }

  function statCardHtml(label, value) {
    return `
      <div class="nai-job-stat">
        <div class="nai-job-stat-label">${escapeHtml(label)}</div>
        <div class="nai-job-stat-value">${escapeHtml(value)}</div>
      </div>
    `;
  }

  function keyStatusSummary() {
    const entries = state.keyPool;
    const unused = entries.filter((entry) => !entry.attemptedAt).length;
    const creating = entries.filter((entry) => entry.statusText === '创建中').length;
    const created = entries.filter((entry) => entry.channelId || entry.channelCreatedAt).length;
    const failed = entries.filter((entry) => entry.statusText === '创建失败').length;
    const alive = entries.filter((entry) => (entry.channelId || entry.channelCreatedAt) && Number(entry.status) === 1).length;
    const dead = entries.filter((entry) => (entry.channelId || entry.channelCreatedAt) && entry.status !== null && Number(entry.status) !== 1).length;
    const attempted = entries.filter((entry) => entry.attemptedAt).length;
    const latestAdded = entries.map((entry) => entry.addedAt).filter(Boolean).sort().at(-1);
    const firstAdded = entries.map((entry) => entry.addedAt).filter(Boolean).sort()[0] || '';
    const batchCount = state.activeJob?.batches?.length || 0;
    const lastBatch = state.activeJob?.batches?.at(-1);
    const durationMs = firstAdded ? Date.now() - Date.parse(firstAdded) : 0;
    return {
      total: entries.length,
      unused,
      creating,
      created,
      failed,
      alive,
      dead,
      attempted,
      latestAdded,
      firstAdded,
      batchCount,
      lastBatch,
      durationMs,
    };
  }

  function updateKeyPoolView() {
    const summary = qs('#nai-keyPoolSummary');
    const updated = qs('#nai-keyPoolUpdated');
    const listPanel = qs('#nai-keyListPanel');
    const statsPanel = qs('#nai-keyStatsPanel');
    const stats = keyStatusSummary();
    if (updated) {
      updated.textContent = stats.latestAdded ? `最近 ${formatLocalDateTime(stats.latestAdded)}` : '未入库';
    }
    if (summary) {
      summary.innerHTML = [
        statCardHtml('总 key', String(stats.total)),
        statCardHtml('未使用', String(stats.unused)),
        statCardHtml('存活', String(stats.alive)),
        statCardHtml('死亡/失败', `${stats.dead}/${stats.failed}`),
      ].join('');
    }
    if (listPanel) {
      const rows = state.keyPool.slice(-80).reverse().map((entry) => `
        <div class="nai-key-row">
          <strong title="${escapeHtml(entry.keyPreview)}">${escapeHtml(entry.keyPreview)}</strong>
          <span>${escapeHtml(entry.statusText || '未使用')}</span>
        </div>
      `).join('');
      listPanel.innerHTML = rows
        ? `<div class="nai-key-list">${rows}</div>`
        : '<div class="nai-bulk-help" style="padding: 10px;">暂无 key。左上粘贴并点击添加入库后会显示在这里。</div>';
    }
    if (statsPanel) {
      const lastBatchText = stats.lastBatch
        ? `#${stats.lastBatch.no} ${formatLocalDateTime(stats.lastBatch.startedAt)}`
        : '-';
      statsPanel.innerHTML = `
        <div class="nai-key-summary">
          ${statCardHtml('总 key', String(stats.total))}
          ${statCardHtml('未使用', String(stats.unused))}
          ${statCardHtml('已使用', String(stats.attempted))}
          ${statCardHtml('存活 key', String(stats.alive))}
          ${statCardHtml('死亡 key', String(stats.dead))}
          ${statCardHtml('创建中', String(stats.creating))}
          ${statCardHtml('已创建', String(stats.created))}
          ${statCardHtml('创建失败', String(stats.failed))}
          ${statCardHtml('批次数', String(stats.batchCount))}
          ${statCardHtml('最近批次', lastBatchText)}
          ${statCardHtml('key 池持续', formatDuration(stats.durationMs))}
          ${statCardHtml('首次入库', formatLocalDateTime(stats.firstAdded))}
          ${statCardHtml('最近入库', formatLocalDateTime(stats.latestAdded))}
        </div>
      `;
    }
  }

  function updateJobStats() {
    const statsEl = qs('#nai-jobStats');
    const batchesEl = qs('#nai-jobBatches');
    updateKeyPoolView();
    if (!statsEl || !batchesEl) return;
    const job = state.activeJob;
    const stats = calculateJobStats(job);
    const runtimeConfigLabel = job ? runtimeConfigSummary(job.runtimeConfig) : '-';
    statsEl.innerHTML = [
      statCardHtml('key 库 / 未使用', `${stats.totalKeys} / ${stats.unusedKeys}`),
      statCardHtml('已尝试 / 已创建', `${stats.attempted} / ${stats.created}`),
      statCardHtml('存活 / 禁用 / 未知', `${stats.alive} / ${stats.disabled} / ${stats.unknown}`),
      statCardHtml('目标 / 阈值 / 补批', runtimeConfigLabel),
      statCardHtml('今日累计添加', String(stats.todayCreated)),
      statCardHtml('批次数', String(stats.batches)),
      statCardHtml('第一个渠道时间', formatLocalDateTime(stats.firstCreated)),
      statCardHtml('作业持续', formatDuration(stats.jobDurationMs)),
      statCardHtml('平均存活', formatDuration(stats.averageLifetimeMs)),
      statCardHtml('总费用/额度', String(stats.totalQuota)),
      statCardHtml('平均费用/额度', stats.averageQuota.toFixed(2)),
      statCardHtml('最高费用/额度', String(stats.maxQuota)),
      statCardHtml('最低费用/额度', String(stats.minQuota)),
    ].join('');

    const batches = job?.batches || [];
    if (!batches.length) {
      batchesEl.innerHTML = '<div class="nai-bulk-help" style="padding: 9px;">暂无批次记录。</div>';
      return;
    }
    batchesEl.innerHTML = batches.slice(-10).reverse().map((batch) => `
      <div class="nai-job-batch-row">
        <span>#${escapeHtml(batch.no)}</span>
        <span>${escapeHtml(formatLocalDateTime(batch.startedAt))}</span>
        <span>${escapeHtml(batch.success)}/${escapeHtml(batch.size)}</span>
        <span>${escapeHtml(batch.reason || '')}</span>
      </div>
    `).join('');
  }

  function syncRuntimeFieldsFromJob(job) {
    if (!job?.runtimeConfig || state.strategyDirty) return;
    setRuntimeCheck('autoRefill', job.runtimeConfig.autoRefill !== false);
    setRuntimeField('targetAliveSize', String(job.runtimeConfig.targetAliveSize));
    setRuntimeField('aliveThreshold', String(job.runtimeConfig.aliveThreshold));
    setRuntimeField('replenishBatchSize', String(job.runtimeConfig.replenishBatchSize));
    setRuntimeField('monitorIntervalSec', String(job.runtimeConfig.monitorIntervalSec));
  }

  function updateJobPreview() {
    const preview = qs('#nai-jobPreview');
    if (!preview) return;
    const job = state.activeJob;
    const panel = document.getElementById(SCRIPT_ID);
    const rightOpen = panel?.getAttribute('data-nai-right-open') !== 'false';
    if (!job && !rightOpen) {
      preview.innerHTML = '';
      return;
    }
    if (!job) {
      const config = collectConfig(false);
      preview.innerHTML = `
        <div class="nai-job-preview-row"><span>预览名称</span><strong>${escapeHtml(resolveJobName(config))}</strong></div>
        <div class="nai-job-preview-row"><span>渠道类型</span><strong>${escapeHtml(channelTypeEntry(config.typePreset)[1])}</strong></div>
        <div class="nai-job-preview-row"><span>策略</span><strong>${escapeHtml(runtimeConfigSummary(collectRuntimeConfig(config)))}</strong></div>
        <div class="nai-job-preview-row"><span>状态</span><strong>尚未创建，右侧输入只会用于新作业。</strong></div>
      `;
      return;
    }
    const snapshot = job.configSnapshot || {};
    preview.innerHTML = `
      <div class="nai-job-preview-row"><span>作业名称</span><strong>${escapeHtml(job.name || job.id)}</strong></div>
      <div class="nai-job-preview-row"><span>作业状态</span><strong>${escapeHtml(job.stopped ? '已结束' : job.paused ? '已暂停' : '监控中')}</strong></div>
      <div class="nai-job-preview-row"><span>创建时间</span><strong>${escapeHtml(formatLocalDateTime(job.startedAt))}</strong></div>
      <div class="nai-job-preview-row"><span>站点</span><strong>${escapeHtml(job.site?.name || '-')} · ${escapeHtml(job.site?.url || '-')}</strong></div>
      <div class="nai-job-preview-row"><span>渠道类型</span><strong>${escapeHtml(channelTypeEntry(snapshot.typePreset || DEFAULT_CONFIG.typePreset)[1])}</strong></div>
      <div class="nai-job-preview-row"><span>分组</span><strong>${escapeHtml(snapshot.group || '-')}</strong></div>
      <div class="nai-job-preview-row"><span>模型</span><strong>${escapeHtml(normalizeList(snapshot.models) || '-')}</strong></div>
      <div class="nai-job-preview-row"><span>策略</span><strong>${escapeHtml(runtimeConfigSummary(job.runtimeConfig))}</strong></div>
    `;
  }

  function updateJobControls() {
    const runButtons = qsa('[data-nai-run]');
    const toggleJob = qs('[data-nai-toggle-job]');
    const refresh = qs('[data-nai-refresh-job]');
    const exportButton = qs('[data-nai-export-job]');
    const applyStrategy = qs('[data-nai-apply-strategy]');
    const openParams = qs('[data-nai-open-params]');
    const jobTitle = qs('#nai-jobTitle');
    const emptyState = qs('#nai-jobEmptyState');
    const runtimeSection = qs('#nai-jobRuntimeSection');
    const hasJob = Boolean(state.activeJob);
    const active = state.activeJob && !state.activeJob.stopped;
    const paused = active && state.activeJob.paused;
    const statusText = qs('#nai-jobStatusText');
    syncRuntimeFieldsFromJob(state.activeJob);
    updateJobPreview();
    if (jobTitle) jobTitle.textContent = state.activeJob?.name || '暂无作业';
    if (emptyState) emptyState.hidden = hasJob;
    if (runtimeSection) runtimeSection.hidden = !hasJob;
    if (statusText) {
      statusText.textContent = !state.activeJob
        ? '未开始'
        : state.activeJob.stopped
          ? '已结束'
          : paused
            ? '已暂停'
            : state.running
              ? '批次执行中'
              : '监控中';
    }
    runButtons.forEach((run) => {
      run.disabled = state.running;
      run.textContent = state.running ? '添加中...' : '保存创建作业';
    });
    if (toggleJob) {
      toggleJob.disabled = state.running || !active;
      toggleJob.textContent = !hasJob
        ? '开启/暂停作业'
        : paused
          ? '开启作业'
          : active
            ? '暂停作业'
            : '作业已结束';
    }
    if (refresh) refresh.disabled = !state.activeJob || state.monitorBusy || state.running;
    if (exportButton) exportButton.disabled = !state.activeJob;
    if (openParams) openParams.textContent = hasJob ? '查看/新建参数' : '创建作业参数';
    if (applyStrategy) {
      applyStrategy.disabled = !state.activeJob || state.activeJob.stopped || !state.strategyDirty;
      applyStrategy.setAttribute('data-dirty', String(Boolean(state.strategyDirty && state.activeJob && !state.activeJob.stopped)));
    }
  }

  function updateEntryFromChannel(entry, channel) {
    if (!channel) return;
    const previousStatus = entry.status;
    entry.channelId = channel.id ?? entry.channelId;
    entry.channelName = channel.name || entry.channelName;
    entry.channelCreatedAt = channelTimeToIso(channel.created_time ?? channel.createdTime, entry.channelCreatedAt || entry.usedAt || nowIso());
    entry.status = Number.isFinite(Number(channel.status)) ? Number(channel.status) : entry.status;
    entry.statusText = statusLabel(entry.status);
    entry.usedQuota = numericQuota(channel);
    entry.lastSeenAt = nowIso();
    if (entry.status !== null && Number(entry.status) !== 1 && !entry.disabledAt) {
      entry.disabledAt = nowIso();
    }
    if (Number(previousStatus) === 1 && Number(entry.status) !== 1 && !entry.disabledAt) {
      entry.disabledAt = nowIso();
    }
  }

  async function findChannelByName(config, name, type) {
    const params = new URLSearchParams({
      keyword: String(name || ''),
      id_sort: 'true',
    });
    let channels = [];
    try {
      const result = await apiRequest(apiUrl(config, `/search?${params.toString()}`));
      if (!result?.success) throw new Error(result?.message || '搜索渠道失败');
      channels = channelsFromListResult(result);
    } catch {
      const fallbackParams = new URLSearchParams({
        p: '1',
        page_size: String(TEMPLATE_PAGE_SIZE),
        type: String(type),
        id_sort: 'true',
      });
      const result = await apiRequest(apiUrl(config, `?${fallbackParams.toString()}`));
      if (!result?.success) throw new Error(result?.message || '渠道列表回查失败');
      channels = channelsFromListResult(result);
    }
    return channels.find((channel) => channel.name === name && Number(channel.type) === Number(type)) ||
      channels.find((channel) => channel.name === name) ||
      null;
  }

  async function readChannelForEntry(config, entry) {
    if (entry.channelId) {
      const result = await apiRequest(apiUrl(config, `/${encodeURIComponent(entry.channelId)}`));
      if (!result?.success) throw new Error(result?.message || '读取渠道失败');
      return normalizeChannelResult(result);
    }
    if (!entry.channelName) return null;
    return findChannelByName(config, entry.channelName, getChannelType(config));
  }

  async function refreshJobStatuses(job, config) {
    const tracked = (job?.keys || []).filter((entry) => entry.channelId || entry.channelName);
    let refreshed = 0;
    for (const entry of tracked) {
      try {
        const channel = await readChannelForEntry(config, entry);
        if (channel) {
          updateEntryFromChannel(entry, channel);
          refreshed += 1;
        }
      } catch (err) {
        entry.error = err.message;
      }
    }
    updateJobStats();
    return refreshed;
  }

  async function createAutoBatch(job, config, batchSize, reason) {
    const selected = availableEntries(job).slice(0, batchSize);
    if (!selected.length) {
      if (!job.noKeyLogged) {
        recordJobLog(job, 'key 库已无未使用 key，自动补货暂停等待追加。');
        job.noKeyLogged = true;
      }
      updateJobStats();
      return 0;
    }
    job.noKeyLogged = false;

    const batch = {
      no: job.batches.length + 1,
      reason,
      startedAt: nowIso(),
      endedAt: null,
      size: selected.length,
      success: 0,
      failed: 0,
      channelIds: [],
    };
    job.batches.push(batch);
    recordJobLog(job, `开始第 ${batch.no} 批：${reason}，计划 ${selected.length} 个。`);

    const rows = buildRowsForKeys(config, selected.map((entry) => entry.key), job.nextIndex || 0);
    job.nextIndex = (job.nextIndex || 0) + rows.length;
    const delay = Math.max(0, Number.parseInt(config.delayMs || '0', 10) || 0);

    for (let i = 0; i < rows.length; i += 1) {
      if (job.stopped || job.paused) break;
      const entry = selected[i];
      const row = rows[i];
      entry.attemptedAt = nowIso();
      entry.usedAt = entry.attemptedAt;
      entry.channelName = row.name;
      entry.batchNo = batch.no;
      entry.statusText = '创建中';
      updateJobStats();

      try {
        const result = await apiRequest(apiUrl(config), {
          method: 'POST',
          body: JSON.stringify(buildPayload(row, config)),
        });
        if (!result?.success) throw new Error(result?.message || 'NewAPI 返回 success=false');

        let channel = normalizeChannelResult(result);
        if (!channel?.id) {
          await sleep(180);
          channel = await findChannelByName(config, row.name, getChannelType(config));
        }
        if (channel) {
          updateEntryFromChannel(entry, channel);
          if (entry.channelId) batch.channelIds.push(entry.channelId);
        } else {
          entry.channelCreatedAt = entry.channelCreatedAt || nowIso();
          entry.status = config.status ? 1 : 2;
          entry.statusText = statusLabel(entry.status);
        }
        batch.success += 1;
        recordJobLog(job, `OK 第 ${batch.no} 批 ${i + 1}/${rows.length}: ${row.name} (${keyPreview(row.key)})`);
      } catch (err) {
        batch.failed += 1;
        entry.error = err.message;
        entry.statusText = '创建失败';
        recordJobLog(job, `FAIL 第 ${batch.no} 批 ${i + 1}/${rows.length}: ${row.name} - ${err.message}`, 'error');
        if (!config.continueOnError) break;
      }

      updateJobStats();
      if (delay > 0 && i < rows.length - 1) await sleep(delay);
    }

    batch.endedAt = nowIso();
    recordJobLog(job, `第 ${batch.no} 批完成：成功 ${batch.success}/${batch.size}。`);
    if (batch.success > 0) refreshHostChannelList();
    updateJobStats();
    return batch.success;
  }

  function startMonitorLoop() {
    if (state.monitorTimer) clearInterval(state.monitorTimer);
    const runtime = state.activeJob?.runtimeConfig || collectRuntimeConfig(collectConfig(false));
    const interval = parsePositiveInt(runtime.monitorIntervalSec, 60, 5) * 1000;
    state.monitorTimer = window.setInterval(monitorActiveJob, interval);
    updateJobControls();
  }

  async function monitorActiveJob() {
    const job = state.activeJob;
    if (!job || job.stopped || job.paused || state.monitorBusy) return;
    state.monitorBusy = true;
    updateJobControls();
    const config = configForJob(job);
    try {
      const refreshed = await refreshJobStatuses(job, config);
      const stats = calculateJobStats(job);
      const threshold = parsePositiveInt(config.aliveThreshold, 5, 0);
      recordJobLog(job, `监控刷新 ${refreshed} 个渠道，当前存活 ${stats.alive} 个。`);
      if (config.autoRefill && stats.alive < threshold) {
        const batchSize = parsePositiveInt(config.replenishBatchSize, 10, 1);
        setRunning(true);
        try {
          await createAutoBatch(job, config, batchSize, `存活 ${stats.alive} < ${threshold}`);
        } finally {
          setRunning(false);
        }
      } else if (!config.autoRefill) {
        recordJobLog(job, '自动补货未启用，本次监控只刷新状态。');
      }
    } finally {
      state.monitorBusy = false;
      updateJobStats();
      updateJobControls();
    }
  }

  async function refreshActiveJobStatus() {
    const job = state.activeJob;
    if (!job || state.monitorBusy) return;
    state.monitorBusy = true;
    updateJobControls();
    try {
      const config = configForJob(job);
      const refreshed = await refreshJobStatuses(job, config);
      recordJobLog(job, `手动刷新作业状态：已读取 ${refreshed} 个渠道。`);
    } catch (err) {
      recordJobLog(job, `手动刷新失败：${err.message}`, 'error');
    } finally {
      state.monitorBusy = false;
      updateJobStats();
      updateJobControls();
    }
  }

  function toggleActiveJobRunning() {
    const job = state.activeJob;
    if (!job || job.stopped || state.running) return;
    if (job.paused) {
      resumeActiveJob();
      return;
    }
    stopActiveJob();
  }

  function stopActiveJob() {
    const job = state.activeJob;
    if (!job || job.stopped || job.paused) return;
    if (state.monitorTimer) {
      clearInterval(state.monitorTimer);
      state.monitorTimer = null;
    }
    job.paused = true;
    recordJobLog(job, '已暂停自动监控。');
    updateJobStats();
    updateJobControls();
  }

  function resumeActiveJob() {
    const job = state.activeJob;
    if (!job || job.stopped || !job.paused) return;
    job.paused = false;
    recordJobLog(job, '已继续自动监控。');
    startMonitorLoop();
    monitorActiveJob();
    updateJobStats();
    updateJobControls();
  }

  function sanitizedJobForExport(job, includeRawKeys = false) {
    if (!job) return null;
    return {
      ...job,
      keys: job.keys.map(({ key, ...entry }) => ({
        ...entry,
        keyMasked: keyPreview(key),
        ...(includeRawKeys ? { key } : {}),
      })),
    };
  }

  function exportActiveJob() {
    const job = state.activeJob;
    if (!job) return;
    const includeRawKeys = collectConfig(false).exportRawKeys === true;
    const payload = JSON.stringify(sanitizedJobForExport(job, includeRawKeys), null, 2);
    downloadJson(payload, `newapi-bulk-job-${new Date().toISOString().replace(/[:.]/g, '-')}.json`);
    recordJobLog(job, '已导出本次作业日志。');
  }

  function downloadJson(payload, filename) {
    const blob = new Blob([payload], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = filename;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  function exportWorkspace() {
    persistWorkspaceState();
    const payload = JSON.stringify(workspacePayload(), null, 2);
    downloadJson(payload, `newapi-bulk-work-${new Date().toISOString().replace(/[:.]/g, '-')}.json`);
    appendLog('已导出完整工作记录。');
  }

  function importWorkspaceFromFile(event) {
    const file = event.target?.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const payload = JSON.parse(String(reader.result || '{}'));
        const ok = window.confirm('导入工作会替换当前 key 池、作业和日志。确认继续？');
        if (!ok) return;
        applyWorkspacePayload(payload, { keepMonitor: false });
        persistWorkspaceState();
        renderWorkLog();
        updateJobStats();
        updateJobControls();
        appendLog(`已导入工作记录：${file.name}`);
        if (state.activeJob && !state.activeJob.stopped && !state.activeJob.paused) {
          startMonitorLoop();
        }
      } catch (err) {
        appendLog(`导入工作失败：${err.message}`, 'error');
      } finally {
        event.target.value = '';
      }
    };
    reader.onerror = () => {
      appendLog(`导入工作失败：${reader.error?.message || '文件读取失败'}`, 'error');
      event.target.value = '';
    };
    reader.readAsText(file);
  }

  function resetWorkspace() {
    const ok = window.confirm('确认重置当前工作？这会清空 key 池、作业、批次和日志，并停止监控。');
    if (!ok) return;
    if (state.monitorTimer) {
      clearInterval(state.monitorTimer);
      state.monitorTimer = null;
    }
    state.keyPool = [];
    state.keyPoolSet = new Set();
    state.activeJob = null;
    state.workLogs = [];
    state.strategyDirty = false;
    state.monitorBusy = false;
    state.running = false;
    state.nameSeedKey = '';
    state.nameTimestamp = '';
    state.nameDate = '';
    state.randomCodes = new Map();
    localStorage.removeItem(WORKSPACE_STORAGE_KEY);
    resetFormToDefaults();
    const keyInput = qs('#nai-keys');
    if (keyInput) keyInput.value = '';
    setParamsPaneOpen(false);
    renderWorkLog();
    refreshPreview();
    updateJobStats();
    updateJobControls();
  }

  function resetFormToDefaults() {
    const config = cloneValue(DEFAULT_CONFIG);
    for (const id of fieldIds) {
      setField(id, config[id] ?? '');
    }
    for (const id of checkboxIds) {
      setCheck(id, config[id] === true);
    }
    setRuntimeCheck('autoRefill', config.autoRefill === true);
    setRuntimeField('targetAliveSize', config.targetAliveSize);
    setRuntimeField('aliveThreshold', config.aliveThreshold);
    setRuntimeField('replenishBatchSize', config.replenishBatchSize);
    setRuntimeField('monitorIntervalSec', config.monitorIntervalSec);
    renderNameEditor(config);
    saveConfig(config);
    updateTypePicker();
    updateBaseUrlDisplay();
    updateGroupSelectFromInput();
  }

  function updateSiteInfo() {
    const site = currentSiteInfo();
    const name = qs('#nai-siteName');
    const url = qs('#nai-siteUrl');
    if (name) name.textContent = site.name;
    if (url) url.textContent = site.url;
  }

  function updateBaseUrlDisplay() {
    const display = qs('#nai-baseUrlDisplay');
    if (!display) return;
    let type = DEFAULT_CONFIG.typePreset;
    try {
      type = getChannelType(collectConfig(false));
    } catch {
      /* keep default type */
    }
    const baseUrl = defaultBaseUrlForType(type);
    display.textContent = baseUrl || baseUrlDisplayValue(type);
    display.setAttribute('data-empty', String(!baseUrl));
  }

  function updateTemplateOptions(channels, selected = '') {
    state.templates = channels;
    state.templatesLoaded = true;
    const select = qs('#nai-templateSelect');
    if (select) {
      select.innerHTML = renderTemplateOptions(channels, selected);
      if (selected) select.value = String(selected);
    }
    const help = qs('#nai-template-help');
    if (help) {
      help.textContent = channels.length
        ? `已读取 ${channels.length} 个同类型样板渠道。`
        : `当前类型暂无样板渠道；先手动创建一个后再刷新。`;
    }
  }

  async function loadTemplates() {
    const config = collectConfig(false);
    let type;
    try {
      type = getChannelType(config);
    } catch (err) {
      appendLog(err.message, 'error');
      return;
    }

    const select = qs('#nai-templateSelect');
    const previousSelected = select?.value || '';
    if (select) {
      select.innerHTML = '<option value="">读取中...</option>';
      select.disabled = true;
    }

    const params = new URLSearchParams({
      p: '1',
      page_size: String(TEMPLATE_PAGE_SIZE),
      type: String(type),
      id_sort: 'true',
    });

    try {
      const result = await apiRequest(apiUrl(config, `?${params.toString()}`));
      if (!result?.success) throw new Error(result?.message || '读取失败');
      const channels = channelsFromListResult(result);
      const selected = channels.some((channel) => String(channel.id) === String(previousSelected))
        ? previousSelected
        : '';
      updateTemplateOptions(channels, selected);
      appendLog(`已刷新 type=${type} 的样板渠道下拉，共 ${channels.length} 个。`);
    } catch (err) {
      updateTemplateOptions([]);
      appendLog(`刷新样板渠道失败：${err.message}`, 'error');
    } finally {
      if (select) select.disabled = false;
    }
  }

  function normalizeGroupsResult(result) {
    const data = result?.data ?? result;
    let groups = [];
    if (Array.isArray(data)) {
      groups = data;
    } else if (Array.isArray(data?.items)) {
      groups = data.items;
    } else if (data && typeof data === 'object') {
      groups = Object.keys(data);
    }
    return groups
      .map((group) => String(group || '').trim())
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b));
  }

  function updateGroupOptions(groups) {
    state.groups = groups;
    state.groupsLoaded = true;
    const select = qs('#nai-groupSelect');
    const currentGroup = qs('#nai-group')?.value || '';
    if (select) {
      select.innerHTML = renderGroupOptions(groups, currentGroup);
      select.disabled = false;
    }
    const menu = qs('[data-nai-group-menu]');
    if (menu) menu.innerHTML = renderGroupMenuOptions(groups, currentGroup);
    updateGroupTriggerText();
    const help = qs('#nai-group-help');
    if (help) {
      help.textContent = groups.length
        ? `已读取 ${groups.length} 个分组。点击下拉可多选，已选项会显示在分组选择框内。`
        : '未读取到分组，可刷新后重试。';
    }
  }

  function setGroupPickerOpen(open) {
    const trigger = qs('[data-nai-group-trigger]');
    const menu = qs('[data-nai-group-menu]');
    if (!trigger || !menu) return;
    trigger.setAttribute('aria-expanded', String(open));
    menu.hidden = !open;
  }

  function updateGroupTriggerText() {
    const text = qs('[data-nai-group-trigger-text]');
    if (text) text.textContent = groupTriggerLabel(qs('#nai-group')?.value || '');
  }

  function updateGroupSelectFromInput() {
    const select = qs('#nai-groupSelect');
    const selectedGroups = new Set(selectedGroupsFromValue(qs('#nai-group')?.value || '', state.groups));
    if (select) {
      Array.from(select.options).forEach((option) => {
        option.selected = selectedGroups.has(option.value);
      });
    }
    qsa('[data-nai-group-option]').forEach((option) => {
      const selected = selectedGroups.has(option.getAttribute('data-nai-group-option') || '');
      option.setAttribute('aria-selected', String(selected));
      const check = qs('input', option);
      if (check) check.checked = selected;
    });
    updateGroupTriggerText();
  }

  function applySelectedGroup() {
    const select = qs('#nai-groupSelect');
    if (!select) return;
    const groups = Array.from(select.selectedOptions)
      .map((option) => String(option.value || '').trim())
      .filter(Boolean);
    setField('group', groups.join(','));
    saveConfig(collectConfig(false));
    refreshPreview();
    updateGroupSelectFromInput();
  }

  function toggleGroupOption(group) {
    const normalizedGroup = String(group || '').trim();
    if (!normalizedGroup) return;
    const selected = new Set(normalizeList(qs('#nai-group')?.value || '').split(',').filter(Boolean));
    if (selected.has(normalizedGroup)) {
      selected.delete(normalizedGroup);
    } else {
      selected.add(normalizedGroup);
    }
    setField('group', Array.from(selected).join(','));
    saveConfig(collectConfig(false));
    updateGroupSelectFromInput();
    refreshPreview();
    updateJobPreview();
  }

  async function loadGroups() {
    const select = qs('#nai-groupSelect');
    if (select) {
      select.innerHTML = '<option value="">读取中...</option>';
      select.disabled = true;
    }
    try {
      const result = await apiRequest(GROUPS_API);
      if (!result?.success) throw new Error(result?.message || '读取失败');
      const groups = normalizeGroupsResult(result);
      updateGroupOptions(groups);
      const currentGroup = String(qs('#nai-group')?.value || '').trim();
      if (!currentGroup && groups.length > 0) {
        setField('group', groups.includes('default') ? 'default' : groups[0]);
        saveConfig(collectConfig(false));
        updateGroupSelectFromInput();
        refreshPreview();
      }
      appendLog(`已读取 ${groups.length} 个分组。`);
    } catch (err) {
      updateGroupOptions([]);
      appendLog(`分组读取失败，请刷新后重试：${err.message}`, 'error');
    }
  }

  function renderWorkLog() {
    const log = qs('#nai-log');
    if (!log) return;
    if (!state.workLogs.length) {
      log.textContent = 'Ready.';
      return;
    }
    log.textContent = state.workLogs.map((entry) => {
      const time = new Date(entry.at || Date.now()).toLocaleTimeString();
      return `[${time}] ${entry.message}`;
    }).join('\n');
    log.scrollTop = log.scrollHeight;
  }

  function appendLog(message, kind = '') {
    state.workLogs.push({
      at: nowIso(),
      kind: kind || 'info',
      message: String(message || ''),
    });
    if (state.workLogs.length > 1200) state.workLogs = state.workLogs.slice(-1200);
    renderWorkLog();
    persistWorkspaceState();
  }

  function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  function setRunning(running) {
    state.running = running;
    qsa([
      '[data-nai-run]',
      '[data-nai-load-template]',
      '[data-nai-refresh-templates]',
      '[data-nai-refresh-site]',
      '[data-nai-refresh-groups]',
      '[data-nai-refresh-base-url]',
      '[data-nai-add-keys]',
      '[data-nai-import-work]',
      '[data-nai-reset-work]',
      '[data-nai-preview]',
      '[data-nai-copy-payload]',
    ].join(', ')).forEach((button) => {
      button.disabled = running;
    });
    qsa('[data-nai-run]').forEach((button) => {
      button.textContent = running ? '添加中...' : '保存创建作业';
    });
    updateJobControls();
  }

  function elementLabel(el) {
    return [
      el.textContent,
      el.getAttribute('aria-label'),
      el.getAttribute('title'),
      el.getAttribute('data-title'),
    ]
      .filter(Boolean)
      .join(' ')
      .trim();
  }

  function findHostRefreshButton() {
    const panel = document.getElementById(SCRIPT_ID);
    const controls = qsa('button, [role="button"], a');
    return controls.find((el) => {
      if (panel?.contains(el)) return false;
      if (el.classList?.contains('nai-bulk-button')) return false;
      if (el.disabled || el.getAttribute('aria-disabled') === 'true') return false;
      const label = elementLabel(el);
      if (!/(^|\s)(刷新|Refresh|Reload)(\s|$)/i.test(label)) return false;
      return !/(余额|Balance|凭证|Credential|模型|Model|详情|Details|全部|All|上游|Upstream)/i.test(label);
    });
  }

  function refreshHostChannelList() {
    const detail = { source: SCRIPT_ID, at: Date.now() };
    window.dispatchEvent(new CustomEvent('newapi:channels-created', { detail }));
    document.dispatchEvent(new CustomEvent('newapi:channels-created', { detail }));
    window.dispatchEvent(new Event('focus'));

    const refreshButton = findHostRefreshButton();
    if (refreshButton) {
      window.setTimeout(() => refreshButton.click(), 80);
      return 'button';
    }
    return 'event';
  }

  async function runAutoImport(config) {
    const targetAliveSize = parsePositiveInt(config.targetAliveSize, 10, 0);
    const firstBatchSize = Math.max(1, targetAliveSize);
    const selected = availableEntries({ keys: state.keyPool }).slice(0, firstBatchSize);
    const rows = buildRowsForKeys(config, selected.map((entry) => entry.key), 0);

    try {
      validateJobConfig(config, rows);
    } catch (err) {
      appendLog(err.message, 'error');
      refreshPreview();
      return;
    }

    if (selected.length > 0 && !getUserId()) {
      appendLog('未读取到登录用户 ID。新版需 localStorage.uid，v0.13.2 需 localStorage.user.id；请确认已登录并刷新页面。', 'error');
      return;
    }

    const jobName = resolveJobName(config);
    const firstBatchText = selected.length
      ? `首次按当前可用 key 创建 ${selected.length} 个渠道`
      : '当前 key 库无可用 key，创建后等待补 key';
    const ok = window.confirm(`准备创建作业“${jobName}”。${firstBatchText}，存活低于 ${config.aliveThreshold} 时补充 ${config.replenishBatchSize} 个。确认继续？`);
    if (!ok) return;

    state.activeJob = createJob(config);
    state.strategyDirty = false;
    recordJobLog(state.activeJob, `作业“${state.activeJob.name}”启动，key 池 ${state.keyPool.length} 个，保活 ${targetAliveSize}，低于 ${config.aliveThreshold} 补 ${config.replenishBatchSize}，监控间隔 ${config.monitorIntervalSec} 秒。`);
    if (!selected.length) {
      recordJobLog(state.activeJob, '当前 key 库无可用 key，作业已创建并等待补 key。');
      startMonitorLoop();
      updateJobStats();
      updateJobControls();
      return;
    }
    setRunning(true);
    try {
      await createAutoBatch(state.activeJob, configForJob(state.activeJob), firstBatchSize, '保活首批');
    } finally {
      setRunning(false);
      if (!state.activeJob.stopped) startMonitorLoop();
      updateJobStats();
      updateJobControls();
      if (!state.activeJob.stopped) {
        recordJobLog(state.activeJob, `已进入自动监控，间隔 ${parsePositiveInt(config.monitorIntervalSec, 60, 5)} 秒。`);
      }
    }
  }

  async function runImport() {
    if (state.running) return;
    const config = collectConfig(true);
    if (state.activeJob && !state.activeJob.stopped) {
      const ok = window.confirm('当前已有作业。确认按右侧当前输入新建作业，并停止当前作业的监控吗？');
      if (!ok) return;
      recordJobLog(state.activeJob, '用户选择按新输入新建作业，当前作业停止监控。');
      state.activeJob.stopped = true;
      state.activeJob.stoppedAt = nowIso();
      if (state.monitorTimer) {
        clearInterval(state.monitorTimer);
        state.monitorTimer = null;
      }
    }
    await runAutoImport(config);
  }

  async function copyFirstPayload() {
    const config = collectConfig(true);
    const rows = buildRows(config);
    try {
      validateConfig(config, rows);
      const payload = buildPayload(rows[0], config);
      await navigator.clipboard.writeText(JSON.stringify(payload, null, 2));
      appendLog('已复制首条 payload。');
    } catch (err) {
      appendLog(err.message, 'error');
    }
  }

  function applyTemplateChannel(channel) {
    if (!channel) throw new Error('没有读取到渠道数据');
    setField('typePreset', CHANNEL_TYPES.some(([value]) => value === channel.type) ? String(channel.type) : DEFAULT_CONFIG.typePreset);
    setField('models', channel.models || '');
    setField('group', channel.group || 'default');
    setField('modelMapping', prettyJsonString(channel.model_mapping));
    setField('priority', String(channel.priority ?? 0));
    setField('weight', String(channel.weight ?? 0));
    setField('tag', channel.tag || '');
    setField('remark', channel.remark || '');
    setField('settingJson', prettyJsonString(channel.setting) || DEFAULT_SETTING_JSON);
    setField('settingsJson', prettyJsonString(channel.settings) || '{}');
    setField('paramOverride', prettyJsonString(channel.param_override));
    setField('headerOverride', prettyJsonString(channel.header_override));
    setField('statusCodeMapping', prettyJsonString(channel.status_code_mapping));
    setField('other', channel.other || '');
    setCheck('status', channel.status === 1);
    setCheck('autoBan', channel.auto_ban !== 0);

    try {
      const settings = channel.settings ? JSON.parse(channel.settings) : {};
      setCheck('allowServiceTier', settings.allow_service_tier === true);
      setCheck('allowInferenceGeo', settings.allow_inference_geo === true);
      setCheck('allowSpeed', settings.allow_speed === true);
      setCheck('claudeBetaQuery', settings.claude_beta_query === true);
    } catch {
      /* keep current checkboxes */
    }

    saveConfig(collectConfig(false));
    updateGroupSelectFromInput();
    updateBaseUrlDisplay();
    refreshPreview();
    updateJobPreview();
    if (channel.base_url) {
      appendLog(`样板渠道有自定义 API 地址 ${channel.base_url}；批量创建仍会留空，使用 NewAPI 内置默认地址。`);
    }
  }

  function prettyJsonString(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return '';
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      return trimmed;
    }
  }

  function setField(id, value) {
    const el = qs(`[data-nai-field="${id}"]`);
    if (el) el.value = value;
    if (id === 'typePreset') updateTypePicker();
  }

  function setCheck(id, checked) {
    const el = qs(`[data-nai-check="${id}"]`);
    if (el) el.checked = checked;
  }

  function setRuntimeField(id, value) {
    const el = qs(`[data-nai-runtime-field="${id}"]`);
    if (el) el.value = value;
  }

  function setRuntimeCheck(id, checked) {
    const el = qs(`[data-nai-runtime-check="${id}"]`);
    if (el) el.checked = checked;
  }

  async function loadSelectedTemplate() {
    const config = collectConfig(false);
    const id = String(qs('#nai-templateSelect')?.value || '').trim();
    if (!id) {
      appendLog('请先选择一个样板渠道。', 'error');
      return;
    }
    try {
      let channel = state.templates.find((item) => String(item.id) === id);
      if (!channel) {
        const result = await apiRequest(apiUrl(config, `/${encodeURIComponent(id)}`));
        if (!result?.success) throw new Error(result?.message || '读取失败');
        channel = normalizeChannelResult(result);
      }
      applyTemplateChannel(channel);
      appendLog(`已读取样板渠道 #${channel.id} 的模型/映射。`);
    } catch (err) {
      appendLog(`读取样板渠道 #${id} 失败：${err.message}`, 'error');
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', mount, { once: true });
  } else {
    mount();
  }
})();
