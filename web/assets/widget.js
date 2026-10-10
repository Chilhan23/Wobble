(function (window, document) {
  'use strict';

  if (window.WobbleHelpdesk) return;

  const styles = `
    #wobble-widget-root {
      position: fixed;
      bottom: 24px;
      right: 24px;
      z-index: 999999;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      font-size: 14px;
      color: #1e293b;
    }
    #wobble-widget-root * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
    .wobble-launcher {
      width: 58px;
      height: 58px;
      border-radius: 50%;
      background: linear-gradient(135deg, #0284c7 0%, #0369a1 100%);
      box-shadow: 0 8px 24px rgba(2, 132, 199, 0.4);
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: pointer;
      color: #ffffff;
      border: none;
      outline: none;
      transition: transform 0.2s cubic-bezier(0.34, 1.56, 0.64, 1), box-shadow 0.2s ease;
      position: relative;
    }
    .wobble-launcher:hover {
      transform: scale(1.06);
      box-shadow: 0 10px 28px rgba(2, 132, 199, 0.5);
    }
    .wobble-launcher svg {
      width: 28px;
      height: 28px;
      fill: currentColor;
      transition: transform 0.2s ease;
    }
    .wobble-launcher.active svg.icon-chat { display: none; }
    .wobble-launcher:not(.active) svg.icon-close { display: none; }
    .wobble-badge {
      position: absolute;
      top: -2px;
      right: -2px;
      background: #ef4444;
      color: #ffffff;
      font-size: 11px;
      font-weight: 700;
      min-width: 20px;
      height: 20px;
      border-radius: 10px;
      display: flex;
      align-items: center;
      justify-content: center;
      border: 2px solid #ffffff;
    }

    .wobble-box {
      display: none;
      position: absolute;
      bottom: 72px;
      right: 0;
      width: 380px;
      max-width: calc(100vw - 32px);
      height: 560px;
      max-height: calc(100vh - 110px);
      background: #ffffff;
      border-radius: 18px;
      box-shadow: 0 16px 40px rgba(0, 0, 0, 0.16);
      border: 1px solid #e2e8f0;
      flex-direction: column;
      overflow: hidden;
      transform-origin: bottom right;
      animation: wobblePop 0.25s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .wobble-box.open { display: flex; }
    @keyframes wobblePop {
      from { opacity: 0; transform: scale(0.92) translateY(10px); }
      to { opacity: 1; transform: scale(1) translateY(0); }
    }

    .wobble-header {
      background: linear-gradient(135deg, #0284c7 0%, #0369a1 100%);
      color: #ffffff;
      padding: 16px 18px;
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    .wobble-header-title {
      font-weight: 700;
      font-size: 15px;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .wobble-status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #94a3b8;
    }
    .wobble-status-dot.online { background: #4ade80; box-shadow: 0 0 8px #4ade80; }
    .wobble-header-sub {
      font-size: 11px;
      color: #bae6fd;
      margin-top: 2px;
    }
    .wobble-btn-close {
      background: transparent;
      border: none;
      color: #ffffff;
      cursor: pointer;
      opacity: 0.8;
      padding: 4px;
    }
    .wobble-btn-close:hover { opacity: 1; }

    .wobble-messages {
      flex: 1;
      overflow-y: auto;
      padding: 16px;
      display: flex;
      flex-direction: column;
      gap: 12px;
      background: #f8fafc;
    }
    .wobble-msg {
      display: flex;
      flex-direction: column;
      max-width: 82%;
    }
    .wobble-msg.user { align-self: flex-end; align-items: flex-end; }
    .wobble-msg.other { align-self: flex-start; align-items: flex-start; }
    .wobble-msg-sender {
      font-size: 10px;
      font-weight: 600;
      color: #64748b;
      margin-bottom: 3px;
      padding: 0 2px;
    }
    .wobble-bubble {
      padding: 10px 14px;
      border-radius: 14px;
      font-size: 13px;
      line-height: 1.45;
      word-break: break-word;
    }
    .wobble-msg.user .wobble-bubble {
      background: #0284c7;
      color: #ffffff;
      border-bottom-right-radius: 3px;
    }
    .wobble-msg.other .wobble-bubble {
      background: #ffffff;
      color: #1e293b;
      border: 1px solid #e2e8f0;
      border-bottom-left-radius: 3px;
      box-shadow: 0 1px 2px rgba(0,0,0,0.04);
    }
    .wobble-msg img {
      max-width: 100%;
      border-radius: 8px;
      margin-top: 6px;
      cursor: pointer;
    }
    .wobble-notice {
      align-self: center;
      background: #e0f2fe;
      color: #0369a1;
      font-size: 11px;
      padding: 6px 12px;
      border-radius: 999px;
      text-align: center;
      margin: 4px 0;
    }
    .wobble-typing {
      display: none;
      align-self: flex-start;
      font-size: 11px;
      color: #64748b;
      font-style: italic;
      padding: 4px 8px;
    }

    .wobble-footer {
      padding: 12px 14px;
      background: #ffffff;
      border-top: 1px solid #e2e8f0;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .wobble-input {
      flex: 1;
      border: 1px solid #cbd5e1;
      border-radius: 20px;
      padding: 9px 14px;
      font-size: 13px;
      outline: none;
      color: #1e293b;
      background: #ffffff;
    }
    .wobble-input:focus { border-color: #0284c7; }
    .wobble-btn-send {
      width: 36px;
      height: 36px;
      border-radius: 50%;
      background: #0284c7;
      color: #ffffff;
      border: none;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      flex-shrink: 0;
    }
    .wobble-btn-send:hover { background: #0369a1; }
    .wobble-btn-send:disabled { opacity: 0.5; cursor: not-allowed; }
    .wobble-attach-label {
      cursor: pointer;
      color: #64748b;
      padding: 6px;
      display: flex;
      align-items: center;
    }
    .wobble-attach-label:hover { color: #0284c7; }

    .wobble-rate-overlay {
      display: none;
      position: absolute;
      inset: 0;
      background: rgba(15, 23, 42, 0.85);
      backdrop-filter: blur(4px);
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 24px;
      color: #ffffff;
      text-align: center;
      z-index: 10;
    }
    .wobble-stars {
      display: flex;
      gap: 8px;
      font-size: 28px;
      margin: 16px 0;
      cursor: pointer;
    }
    .wobble-star { color: #64748b; transition: color 0.15s; }
    .wobble-star.active { color: #fbbf24; }
  `;

  function init() {
    const config = window.WobbleConfig || window.TechNovaConfig || {};
    let gatewayUrl = (config.gatewayUrl || '').replace(/\/$/, '');
    let token = config.token || '';
    let ws = null;
    let selectedRating = 5;
    let unreadCount = 0;
    let isBoxOpen = false;

    const styleEl = document.createElement('style');
    styleEl.innerHTML = styles;
    document.head.appendChild(styleEl);

    const root = document.createElement('div');
    root.id = 'wobble-widget-root';
    root.innerHTML = `
      <div class="wobble-box" id="wobbleBox">
        <div class="wobble-header">
          <div>
            <div class="wobble-header-title">
              <span class="wobble-status-dot" id="wobbleWsDot"></span>
              <span>Bantuan & Dukungan IT</span>
            </div>
            <div class="wobble-header-sub" id="wobbleHeaderSub">Live Support • SIMRS</div>
          </div>
          <button class="wobble-btn-close" id="wobbleCloseBtn">✕</button>
        </div>

        <div class="wobble-messages" id="wobbleMessages">
          <div class="wobble-notice">Sesi helpdesk dimulai. Sampaikan kendala Anda.</div>
        </div>

        <div class="wobble-typing" id="wobbleTyping">🤖 AI Assistant sedang mengetik...</div>

        <div class="wobble-footer">
          <label class="wobble-attach-label" title="Lampirkan Gambar">
            📎
            <input type="file" id="wobbleFileInput" accept="image/*,video/*" style="display:none;" />
          </label>
          <input type="text" class="wobble-input" id="wobbleMsgInput" placeholder="Ketik pesan..." />
          <button class="wobble-btn-send" id="wobbleSendBtn">➤</button>
        </div>

        <div class="wobble-rate-overlay" id="wobbleRateOverlay">
          <div style="font-size: 18px; font-weight: 700;">Tiket Selesai</div>
          <div style="font-size: 12px; color: #bae6fd; margin-top: 4px;">Bagaimana pelayanan IT Support kami?</div>
          <div class="wobble-stars" id="wobbleStars">
            <span class="wobble-star active" data-val="1">★</span>
            <span class="wobble-star active" data-val="2">★</span>
            <span class="wobble-star active" data-val="3">★</span>
            <span class="wobble-star active" data-val="4">★</span>
            <span class="wobble-star active" data-val="5">★</span>
          </div>
          <input type="text" class="wobble-input" id="wobbleRateReview" placeholder="Ulasan (opsional)..." style="width:100%;margin-bottom:12px;" />
          <button class="wobble-btn-send" id="wobbleSubmitRateBtn" style="width:100%;border-radius:10px;height:38px;">Kirim Penilaian</button>
        </div>
      </div>

      <button class="wobble-launcher" id="wobbleLauncher" aria-label="Buka Live Chat">
        <svg class="icon-chat" viewBox="0 0 24 24"><path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2z"/></svg>
        <svg class="icon-close" viewBox="0 0 24 24"><path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
        <span class="wobble-badge" id="wobbleBadge" style="display:none;">0</span>
      </button>
    `;
    document.body.appendChild(root);

    const launcher = document.getElementById('wobbleLauncher');
    const box = document.getElementById('wobbleBox');
    const closeBtn = document.getElementById('wobbleCloseBtn');
    const sendBtn = document.getElementById('wobbleSendBtn');
    const msgInput = document.getElementById('wobbleMsgInput');
    const fileInput = document.getElementById('wobbleFileInput');
    const msgContainer = document.getElementById('wobbleMessages');
    const typingIndicator = document.getElementById('wobbleTyping');
    const wsDot = document.getElementById('wobbleWsDot');
    const badge = document.getElementById('wobbleBadge');
    const rateOverlay = document.getElementById('wobbleRateOverlay');

    function toggleBox() {
      isBoxOpen = !isBoxOpen;
      box.classList.toggle('open', isBoxOpen);
      launcher.classList.toggle('active', isBoxOpen);
      if (isBoxOpen) {
        unreadCount = 0;
        badge.style.display = 'none';
        msgContainer.scrollTop = msgContainer.scrollHeight;
        msgInput.focus();
      }
    }

    launcher.addEventListener('click', toggleBox);
    closeBtn.addEventListener('click', toggleBox);

    function connectWS() {
      if (!gatewayUrl || !token) return;
      if (ws) ws.close();

      const wsProtocol = gatewayUrl.startsWith('https') ? 'wss:' : 'ws:';
      const wsHost = gatewayUrl.replace(/^https?:\/\//, '');
      const wsUrl = `${wsProtocol}//${wsHost}/ws?token=${encodeURIComponent(token)}`;

      ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        wsDot.classList.add('online');
      };

      ws.onclose = () => {
        wsDot.classList.remove('online');
        // Auto-reconnect after 3 seconds
        setTimeout(connectWS, 3000);
      };

      ws.onmessage = (evt) => {
        try {
          const payload = JSON.parse(evt.data);
          handleWSEvent(payload);
        } catch (err) {
          console.error('[Wobble WS] parse error:', err);
        }
      };
    }

    function handleWSEvent(payload) {
      const { event, data } = payload;
      if (event === 'helpdesk_new_message') {
        typingIndicator.style.display = 'none';
        appendMessage(data);
        if (!isBoxOpen && data.sender_type !== 'user') {
          unreadCount++;
          badge.innerText = unreadCount;
          badge.style.display = 'flex';
        }
      } else if (event === 'ai_typing') {
        typingIndicator.style.display = data.typing ? 'block' : 'none';
        msgContainer.scrollTop = msgContainer.scrollHeight;
      } else if (event === 'ticket_claimed') {
        appendNotice(`Tiket diambil alih oleh engineer: <b>${escapeHtml(data.programmer_name || 'Tim IT')}</b>`);
      } else if (event === 'ticket_resolved') {
        rateOverlay.style.display = 'flex';
      }
    }

    async function loadHistory() {
      if (!gatewayUrl || !token) return;
      try {
        const res = await fetch(`${gatewayUrl}/api/v1/ticket/messages?limit=50`, {
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!res.ok) return;
        const body = await res.json();
        if (body.messages && body.messages.length > 0) {
          msgContainer.innerHTML = '';
          body.messages.forEach(appendMessage);
        }
      } catch (e) {
        console.warn('[Wobble] load history error:', e);
      }
    }

    function appendMessage(msg) {
      const isUser = msg.sender_type === 'user';
      const el = document.createElement('div');
      el.className = `wobble-msg ${isUser ? 'user' : 'other'}`;

      const senderName = isUser ? 'Anda' : (msg.sender_name || (msg.sender_type === 'ai' ? '🤖 AI Assistant' : '👨‍💻 IT Support'));
      let mediaHtml = '';
      const mediaUrl = (msg.attachment && msg.attachment.url) ? msg.attachment.url : msg.media_url;
      if (mediaUrl) {
        mediaHtml = `<a href="${mediaUrl}" target="_blank"><img src="${mediaUrl}" alt="lampiran" /></a>`;
      }

      el.innerHTML = `
        <div class="wobble-msg-sender">${escapeHtml(senderName)}</div>
        <div class="wobble-bubble">${escapeHtml(msg.message || '')}${mediaHtml}</div>
      `;
      msgContainer.appendChild(el);
      msgContainer.scrollTop = msgContainer.scrollHeight;
    }

    function appendNotice(htmlText) {
      const el = document.createElement('div');
      el.className = 'wobble-notice';
      el.innerHTML = `ℹ️ ${htmlText}`;
      msgContainer.appendChild(el);
      msgContainer.scrollTop = msgContainer.scrollHeight;
    }

    async function sendChat() {
      const text = msgInput.value.trim();
      if (!text || !token) return;
      msgInput.value = '';

      try {
        await fetch(`${gatewayUrl}/api/v1/ticket/messages`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
          },
          body: JSON.stringify({ message: text })
        });
      } catch (err) {
        console.error('[Wobble] send error:', err);
      }
    }

    async function uploadAttachment(file) {
      if (!file || !token) return;
      const formData = new FormData();
      formData.append('file', file);
      formData.append('message', 'Mengirim lampiran media...');

      try {
        await fetch(`${gatewayUrl}/api/v1/ticket/attachments`, {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` },
          body: formData
        });
        fileInput.value = '';
      } catch (err) {
        console.error('[Wobble] upload error:', err);
      }
    }

    sendBtn.addEventListener('click', sendChat);
    msgInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') sendChat();
    });
    fileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files[0]) {
        uploadAttachment(e.target.files[0]);
      }
    });

    // Rating stars interaction
    const starEls = document.querySelectorAll('.wobble-star');
    starEls.forEach(star => {
      star.addEventListener('click', () => {
        selectedRating = parseInt(star.getAttribute('data-val'), 10);
        starEls.forEach((s, idx) => {
          s.classList.toggle('active', idx < selectedRating);
        });
      });
    });

    document.getElementById('wobbleSubmitRateBtn').addEventListener('click', async () => {
      const review = document.getElementById('wobbleRateReview').value;
      try {
        await fetch(`${gatewayUrl}/api/v1/ticket/rate`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
          },
          body: JSON.stringify({ rating: selectedRating, review: review })
        });
      } catch (e) {
        console.error('[Wobble] rating submit error:', e);
      }
      rateOverlay.style.display = 'none';
      appendNotice('Terima kasih atas penilaian Anda!');
    });

    function escapeHtml(str) {
      return (str || '').replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }

    // Public API
    window.WobbleHelpdesk = {
      open: () => { if (!isBoxOpen) toggleBox(); },
      close: () => { if (isBoxOpen) toggleBox(); },
      toggle: toggleBox,
      setToken: (newToken, newGatewayUrl) => {
        token = newToken;
        if (newGatewayUrl) gatewayUrl = newGatewayUrl.replace(/\/$/, '');
        loadHistory();
        connectWS();
      }
    };

    // Auto-init if token is ready
    if (token) {
      loadHistory();
      connectWS();
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})(window, document);
