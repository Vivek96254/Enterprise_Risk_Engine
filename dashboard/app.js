/**
 * Enterprise Risk Engine Dashboard
 * Minimalistic Real-time Analytics Interface
 */

// ============================================
// Configuration
// ============================================

const CONFIG = {
    // When served via nginx proxy, API is at same origin
    // When running standalone on localhost:3000, API is at localhost:8080
    API_BASE: window.location.port === '3000' && window.location.hostname === 'localhost'
        ? 'http://localhost:8080/api/v1'
        : `${window.location.origin}/api/v1`,
    REFRESH_INTERVAL: 30000, // 30 seconds
    TOAST_DURATION: 4000,
};

// ============================================
// State Management
// ============================================

const state = {
    token: localStorage.getItem('auth_token'),
    user: JSON.parse(localStorage.getItem('user') || 'null'),
    currentView: 'overview',
    flaggedPage: 1,
    refreshTimer: null,
};

// ============================================
// API Client
// ============================================

const api = {
    async request(endpoint, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };

        if (state.token) {
            headers['Authorization'] = `Bearer ${state.token}`;
        }

        try {
            const response = await fetch(`${CONFIG.API_BASE}${endpoint}`, {
                ...options,
                headers,
            });

            if (response.status === 401) {
                logout();
                throw new Error('Session expired');
            }

            const data = await response.json();

            if (!response.ok) {
                throw new Error(data.error || 'Request failed');
            }

            return data;
        } catch (error) {
            if (error.name === 'TypeError') {
                updateApiStatus('error');
                throw new Error('Unable to connect to server');
            }
            throw error;
        }
    },

    // Auth
    login: (email, password) => 
        api.request('/auth/login', {
            method: 'POST',
            body: JSON.stringify({ email, password }),
        }),

    // Metrics
    getSystemMetrics: () => api.request('/metrics/system'),
    getRiskSummary: (date) => api.request(`/risk/summary${date ? `?date=${date}` : ''}`),
    getRiskDistribution: (days = 7) => api.request(`/risk/distribution?days=${days}`),
    getTopRules: (days = 7, limit = 10) => api.request(`/risk/rules/top?days=${days}&limit=${limit}`),
    getHourlyVolume: (date) => api.request(`/analytics/volume/hourly${date ? `?date=${date}` : ''}`),

    // Transactions
    getRecentTransactions: (page = 1, pageSize = 20) =>
        api.request(`/transactions/recent?page=${page}&page_size=${pageSize}`),
    getFlaggedTransactions: (page = 1, pageSize = 10) => 
        api.request(`/transactions/flagged?page=${page}&page_size=${pageSize}`),
    getAccountTransactions: (accountId, page = 1, pageSize = 20) =>
        api.request(`/transactions/account/${accountId}?page=${page}&page_size=${pageSize}`),
    
    // Accounts
    getAccounts: () => api.request('/accounts'),
    createAccount: (userId) => api.request('/accounts', {
        method: 'POST',
        body: JSON.stringify({ user_id: userId, account_type: 'standard' }),
    }),
    
    // Create transaction
    createTransaction: (data) => api.request('/transactions', {
        method: 'POST',
        body: JSON.stringify(data),
    }),

    // Experiments
    getExperiments: () => api.request('/experiments'),
    createExperiment: (data) => api.request('/experiments', {
        method: 'POST',
        body: JSON.stringify(data),
    }),
    startExperiment: (id) => api.request(`/experiments/${id}/start`, { method: 'POST' }),
    pauseExperiment: (id) => api.request(`/experiments/${id}/pause`, { method: 'POST' }),
    stopExperiment: (id) => api.request(`/experiments/${id}/stop`, { method: 'POST' }),
    getExperimentResults: (id) => api.request(`/experiments/${id}/results`),
};

// ============================================
// UI Helpers
// ============================================

const $ = (selector) => document.querySelector(selector);
const $$ = (selector) => document.querySelectorAll(selector);

function showToast(message, type = 'info') {
    const container = $('#toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    container.appendChild(toast);

    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateX(20px)';
        setTimeout(() => toast.remove(), 300);
    }, CONFIG.TOAST_DURATION);
}

function formatNumber(num) {
    if (num === null || num === undefined || num === '--') return '--';
    if (typeof num === 'string') num = parseFloat(num);
    if (isNaN(num)) return '--';
    
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toFixed(num % 1 === 0 ? 0 : 2);
}

function formatCurrency(amount, currency = 'USD') {
    return new Intl.NumberFormat('en-US', {
        style: 'currency',
        currency,
    }).format(amount);
}

function formatTime(timestamp) {
    return new Date(timestamp).toLocaleString('en-US', {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
}

function truncateId(id, length = 8) {
    if (!id) return '--';
    return id.substring(0, length) + '...';
}

function copyToClipboard(text, event) {
    if (event) event.stopPropagation();
    navigator.clipboard.writeText(text).then(() => {
        showToast(`Copied: ${text}`, 'success');
    }).catch(() => {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = text;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        showToast(`Copied: ${text}`, 'success');
    });
    hideIdTooltip();
}

// Custom tooltip for IDs
let tooltipEl = null;

function showIdTooltip(event, fullId) {
    if (!tooltipEl) {
        tooltipEl = document.createElement('div');
        tooltipEl.className = 'id-tooltip';
        document.body.appendChild(tooltipEl);
    }
    
    tooltipEl.textContent = fullId;
    tooltipEl.style.display = 'block';
    
    // Position below and to the right of cursor
    const x = event.clientX + 10;
    const y = event.clientY + 15;
    
    tooltipEl.style.left = x + 'px';
    tooltipEl.style.top = y + 'px';
}

function hideIdTooltip() {
    if (tooltipEl) {
        tooltipEl.style.display = 'none';
    }
}

// Make functions available globally
window.copyToClipboard = copyToClipboard;
window.showIdTooltip = showIdTooltip;
window.hideIdTooltip = hideIdTooltip;

function updateApiStatus(status) {
    const indicator = $('#api-status');
    indicator.className = `status-indicator ${status}`;
    indicator.querySelector('span:last-child').textContent = 
        status === 'connected' ? 'Connected' : 
        status === 'error' ? 'Disconnected' : 'Connecting...';
}

function updateCurrentTime() {
    const timeEl = $('#current-time');
    if (timeEl) {
        timeEl.textContent = new Date().toLocaleTimeString('en-US', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
        });
    }
}

// ============================================
// Authentication
// ============================================

async function handleLogin(e) {
    e.preventDefault();
    
    const email = $('#email').value;
    const password = $('#password').value;
    const errorEl = $('#login-error');

    try {
        const data = await api.login(email, password);
        
        state.token = data.token;
        state.user = data.user;
        
        localStorage.setItem('auth_token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        
        showDashboard();
        showToast('Welcome back!', 'success');
    } catch (error) {
        errorEl.textContent = error.message;
        errorEl.classList.add('visible');
    }
}

function logout() {
    state.token = null;
    state.user = null;
    localStorage.removeItem('auth_token');
    localStorage.removeItem('user');
    
    if (state.refreshTimer) {
        clearInterval(state.refreshTimer);
    }
    
    showLoginScreen();
}

function showLoginScreen() {
    $('#login-screen').classList.add('active');
    $('#dashboard-screen').classList.remove('active');
    $('#login-error').classList.remove('visible');
    $('#email').value = '';
    $('#password').value = '';
}

function showDashboard() {
    $('#login-screen').classList.remove('active');
    $('#dashboard-screen').classList.add('active');
    
    // Update user info
    if (state.user) {
        $('#user-email').textContent = state.user.email || 'User';
        $('#user-role').textContent = state.user.role || 'User';
    }
    
    // Initial data load
    loadDashboardData();
    
    // Start refresh timer
    state.refreshTimer = setInterval(loadDashboardData, CONFIG.REFRESH_INTERVAL);
    
    // Update time every second
    setInterval(updateCurrentTime, 1000);
    updateCurrentTime();
}

// ============================================
// View Navigation
// ============================================

function switchView(viewName) {
    state.currentView = viewName;
    
    // Update nav
    $$('.nav-item').forEach(item => {
        item.classList.toggle('active', item.dataset.view === viewName);
    });
    
    // Update view
    $$('.view').forEach(view => {
        view.classList.toggle('active', view.id === `${viewName}-view`);
    });
    
    // Update title
    const titles = {
        overview: 'Overview',
        transactions: 'Transactions',
        flagged: 'Flagged Transactions',
        experiments: 'A/B Testing',
    };
    $('#view-title').textContent = titles[viewName] || viewName;
    
    // Load view-specific data
    switch (viewName) {
        case 'transactions':
            loadRecentTransactions();
            break;
        case 'flagged':
            loadFlaggedTransactions();
            break;
        case 'experiments':
            loadExperiments();
            break;
    }
}

// ============================================
// Data Loading
// ============================================

async function loadDashboardData() {
    try {
        await Promise.all([
            loadSystemMetrics(),
            loadRiskDistribution(),
            loadHourlyVolume(),
            loadTopRules(),
            loadRiskSummary(),
        ]);
        updateApiStatus('connected');
    } catch (error) {
        console.error('Failed to load dashboard data:', error);
        updateApiStatus('error');
    }
}

async function loadSystemMetrics() {
    try {
        const metrics = await api.getSystemMetrics();
        
        $('#metric-tps').textContent = formatNumber(metrics.transactions_per_sec);
        $('#metric-latency').textContent = formatNumber(metrics.avg_processing_time_ms);
        $('#metric-error-rate').textContent = 
            metrics.error_rate !== undefined ? `${(metrics.error_rate * 100).toFixed(1)}%` : '--';
    } catch (error) {
        console.error('Failed to load system metrics:', error);
    }
}

async function loadRiskSummary() {
    try {
        // Fetch summary (for future use) and flagged transactions (for exact count)
        const [summary, flaggedData] = await Promise.all([
            api.getRiskSummary(),
            api.getFlaggedTransactions(1, 1), // we only need pagination.total
        ]);

        // Prefer the exact total returned by the flagged endpoint so it matches the table
        const flaggedFromApi = flaggedData && flaggedData.pagination ? flaggedData.pagination.total : null;
        const flaggedCount = flaggedFromApi != null
            ? flaggedFromApi
            : (summary.flagged_count || 0) + (summary.blocked_count || 0);

        $('#metric-flagged').textContent = formatNumber(flaggedCount);
        $('#flagged-count').textContent = flaggedCount;
    } catch (error) {
        console.error('Failed to load risk summary:', error);
    }
}

async function loadRiskDistribution() {
    try {
        const data = await api.getRiskDistribution(7);
        const levels = data.levels || {};
        const total = data.total || 1;
        
        const updateBar = (level, count) => {
            const percent = (count / total) * 100;
            const bar = $(`.risk-bar[data-level="${level}"] .risk-bar-fill`);
            const value = $(`#risk-${level}`);
            
            if (bar) bar.style.setProperty('--percent', `${percent}%`);
            if (value) value.textContent = formatNumber(count);
        };
        
        updateBar('low', levels.low || 0);
        updateBar('medium', levels.medium || 0);
        updateBar('high', levels.high || 0);
        updateBar('critical', levels.critical || 0);
    } catch (error) {
        console.error('Failed to load risk distribution:', error);
    }
}

async function loadHourlyVolume() {
    try {
        const data = await api.getHourlyVolume();
        const volumes = data.volumes || [];
        const chartEl = $('#volume-chart');
        
        if (!chartEl) return;
        
        // Create 24 hour slots
        const hourlyData = new Array(24).fill(0);
        volumes.forEach(v => {
            if (v.hour >= 0 && v.hour < 24) {
                hourlyData[v.hour] = v.count;
            }
        });
        
        const maxCount = Math.max(...hourlyData, 1);

        const barsHtml = hourlyData.map((count, hour) => {
            const height = (count / maxCount) * 100;
            const tooltip = `Hour ${hour} • ${count} tx`;
            return `<div class="volume-bar" style="height: ${Math.max(height, 4)}%" title="${tooltip}"></div>`;
        }).join('');

        // Simple numeric labels 0-23
        const labelsHtml = hourlyData.map((_, hour) => {
            return `<div class="volume-label">${hour}</div>`;
        }).join('');

        chartEl.innerHTML = `
            <div class="volume-bars">
                ${barsHtml}
            </div>
            <div class="volume-labels">
                ${labelsHtml}
            </div>
        `;
    } catch (error) {
        console.error('Failed to load hourly volume:', error);
    }
}

async function loadTopRules() {
    try {
        const [rulesData, summary] = await Promise.all([
            api.getTopRules(7, 10),
            api.getRiskSummary(),
        ]);

        const rules = rulesData.rules || [];
        const tbody = $('#top-rules-body');
        
        if (!tbody) return;
        
        if (rules.length === 0) {
            tbody.innerHTML = '<tr class="empty-row"><td colspan="3">No rules triggered</td></tr>';
            return;
        }

        // Denominator: how many transactions were actually flagged/blocked
        const flaggedTotal = (summary.flagged_count || 0) + (summary.blocked_count || 0);
        const safeDenom = flaggedTotal > 0 ? flaggedTotal : 1;
        
        tbody.innerHTML = rules.map(rule => {
            // Percentage of flagged/blocked transactions that had this rule
            const rawPercent = (rule.count / safeDenom) * 100;
            const intensity = Math.max(0, Math.min(100, Math.round(rawPercent)));

            return `
            <tr>
                <td><code class="mono">${rule.rule_id}</code></td>
                <td class="mono">${formatNumber(rule.count)}</td>
                <td>
                    <div class="trend-row" title="${intensity}% of flagged/blocked transactions had this rule in last 7 days">
                        <div class="trend-bar">
                            <div class="trend-fill" style="width: ${intensity}%;"></div>
                        </div>
                        <span class="trend-label">${intensity}%</span>
                    </div>
                </td>
            </tr>
        `;
        }).join('');
    } catch (error) {
        console.error('Failed to load top rules:', error);
        const tbody = $('#top-rules-body');
        if (tbody) {
            tbody.innerHTML = '<tr class="empty-row"><td colspan="3">Failed to load</td></tr>';
        }
    }
}

async function loadFlaggedTransactions() {
    const tbody = $('#flagged-body');
    tbody.innerHTML = '<tr class="loading-row"><td colspan="6">Loading...</td></tr>';
    
    try {
        const data = await api.getFlaggedTransactions(state.flaggedPage, 10);
        const transactions = data.transactions || [];
        const pagination = data.pagination || {};
        
        if (transactions.length === 0) {
            tbody.innerHTML = '<tr class="empty-row"><td colspan="6">No flagged transactions</td></tr>';
            return;
        }
        
        tbody.innerHTML = transactions.map(item => {
            const tx = item.transaction || {};
            return `
                <tr>
                    <td>
                        <code class="mono text-sm id-cell" 
                              onmouseenter="showIdTooltip(event, '${tx.id}')"
                              onmouseleave="hideIdTooltip()"
                              onclick="copyToClipboard('${tx.id}', event)">
                            ${truncateId(tx.id)}
                        </code>
                    </td>
                    <td>
                        <code class="mono text-sm id-cell account-link" 
                              onmouseenter="showIdTooltip(event, '${tx.account_id}')"
                              onmouseleave="hideIdTooltip()"
                              onclick="copyToClipboard('${tx.account_id}', event)">
                            ${truncateId(tx.account_id)}
                        </code>
                    </td>
                    <td class="mono">${formatCurrency(tx.amount || 0, tx.currency || 'USD')}</td>
                    <td><span class="mono">${formatNumber(item.risk_score)}</span></td>
                    <td><span class="risk-badge ${item.risk_level || 'low'}">${item.risk_level || 'unknown'}</span></td>
                    <td>
                        <div class="rules-tags">
                            ${(item.rules_triggered || []).slice(0, 3).map(r => 
                                `<span class="rule-tag">${r}</span>`
                            ).join('')}
                            ${(item.rules_triggered || []).length > 3 ? 
                                `<span class="rule-tag">+${item.rules_triggered.length - 3}</span>` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
        
        // Update pagination
        const totalPages = Math.ceil((pagination.total || 0) / (pagination.page_size || 10));
        $('#flagged-page-info').textContent = `Page ${state.flaggedPage} of ${totalPages || 1}`;
        $('#flagged-prev').disabled = state.flaggedPage <= 1;
        $('#flagged-next').disabled = state.flaggedPage >= totalPages;
    } catch (error) {
        console.error('Failed to load flagged transactions:', error);
        tbody.innerHTML = '<tr class="empty-row"><td colspan="6">Failed to load transactions</td></tr>';
    }
}

// Load recent transactions - first try to get flagged, then fall back to showing instructions
async function loadRecentTransactions() {
    const tbody = $('#transactions-body');
    const searchInput = $('#tx-search');
    
    // If there's already a search value, don't override
    if (searchInput && searchInput.value.trim()) {
        return;
    }
    
    if (!tbody) return;
    
    tbody.innerHTML = '<tr class="loading-row"><td colspan="6">Loading recent transactions...</td></tr>';
    
    try {
        // Load all recent transactions across all accounts
        const data = await api.getRecentTransactions(1, 30);
        const transactions = data.transactions || [];
        
        if (searchInput) {
            searchInput.placeholder = 'Search by Account ID...';
        }
        
        if (transactions.length > 0) {
            tbody.innerHTML = transactions.map(tx => `
                <tr>
                    <td>
                        <code class="mono text-sm id-cell" 
                              onmouseenter="showIdTooltip(event, '${tx.id}')"
                              onmouseleave="hideIdTooltip()"
                              onclick="copyToClipboard('${tx.id}', event)">
                            ${truncateId(tx.id)}
                        </code>
                    </td>
                    <td>
                        <code class="mono text-sm id-cell account-link" 
                              onmouseenter="showIdTooltip(event, '${tx.account_id}')"
                              onmouseleave="hideIdTooltip()"
                              onclick="copyToClipboard('${tx.account_id}', event)"
                              ondblclick="searchByAccount('${tx.account_id}')">
                            ${truncateId(tx.account_id)}
                        </code>
                    </td>
                    <td class="mono">${formatCurrency(tx.amount || 0, tx.currency || 'USD')}</td>
                    <td>${tx.merchant || '--'}</td>
                    <td><span class="status-badge ${tx.status || 'pending'}">${tx.status || 'pending'}</span></td>
                    <td class="text-muted text-sm">${formatTime(tx.created_at)}</td>
                </tr>
            `).join('');
            return;
        }
        
        // No transactions found - show helpful message
        tbody.innerHTML = `
            <tr class="empty-row">
                <td colspan="6">
                    <div style="padding: 20px; text-align: center;">
                        <p style="margin-bottom: 10px;">No transactions found. Click <strong>"New Transaction"</strong> to create one!</p>
                        <p style="font-size: 0.75rem; color: var(--text-muted);">
                            Or use the API: <code style="color: var(--accent-primary)">POST /api/v1/transactions</code>
                        </p>
                    </div>
                </td>
            </tr>`;
    } catch (error) {
        console.error('Failed to load recent transactions:', error);
        tbody.innerHTML = `
            <tr class="empty-row">
                <td colspan="6">
                    <div style="padding: 20px; text-align: center;">
                        <p>Failed to load transactions: ${error.message}</p>
                        <p style="font-size: 0.75rem; color: var(--text-muted); margin-top: 10px;">
                            Click <strong>"New Transaction"</strong> to create one
                        </p>
                    </div>
                </td>
            </tr>`;
    }
}

// Helper to search by clicking account ID
function searchByAccount(accountId) {
    $('#tx-search').value = accountId;
    searchTransactions();
}

async function searchTransactions() {
    const searchInput = $('#tx-search');
    const accountId = searchInput.value.trim();
    const tbody = $('#transactions-body');
    const tableHeader = $('#transactions-view .table-header h3');
    
    console.log('Search triggered, accountId:', accountId);
    
    if (!accountId) {
        showToast('Please enter an Account ID to search', 'warning');
        searchInput.focus();
        return;
    }
    
    // Validate UUID format (basic check)
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (!uuidRegex.test(accountId)) {
        showToast('Invalid Account ID format. Expected UUID.', 'error');
        return;
    }
    
    tbody.innerHTML = '<tr class="loading-row"><td colspan="6">Searching...</td></tr>';
    if (tableHeader) tableHeader.textContent = 'Searching...';
    
    try {
        const data = await api.getAccountTransactions(accountId);
        console.log('Search results:', data);
        const transactions = data.transactions || [];
        
        if (tableHeader) {
            tableHeader.textContent = `Transactions for ${accountId.substring(0, 8)}...`;
        }
        
        if (transactions.length === 0) {
            tbody.innerHTML = `<tr class="empty-row"><td colspan="6">No transactions found for account ${accountId.substring(0, 8)}...</td></tr>`;
            showToast('No transactions found', 'info');
            return;
        }
        
        tbody.innerHTML = transactions.map(tx => `
            <tr>
                <td>
                    <code class="mono text-sm id-cell" 
                          onmouseenter="showIdTooltip(event, '${tx.id}')"
                          onmouseleave="hideIdTooltip()"
                          onclick="copyToClipboard('${tx.id}', event)">
                        ${truncateId(tx.id)}
                    </code>
                </td>
                <td>
                    <code class="mono text-sm id-cell account-link" 
                          onmouseenter="showIdTooltip(event, '${tx.account_id}')"
                          onmouseleave="hideIdTooltip()"
                          onclick="copyToClipboard('${tx.account_id}', event)"
                          ondblclick="searchByAccount('${tx.account_id}')">
                        ${truncateId(tx.account_id)}
                    </code>
                </td>
                <td class="mono">${formatCurrency(tx.amount || 0, tx.currency || 'USD')}</td>
                <td>${tx.merchant || '--'}</td>
                <td><span class="status-badge ${tx.status || 'pending'}">${tx.status || 'pending'}</span></td>
                <td class="text-muted text-sm">${formatTime(tx.created_at)}</td>
            </tr>
        `).join('');
        
        showToast(`Found ${transactions.length} transactions`, 'success');
    } catch (error) {
        console.error('Failed to load transactions:', error);
        tbody.innerHTML = `<tr class="empty-row"><td colspan="6">${error.message}</td></tr>`;
        if (tableHeader) tableHeader.textContent = 'Recent Transactions';
        showToast(error.message, 'error');
    }
}

// Make searchByAccount available globally
window.searchByAccount = searchByAccount;

async function loadExperiments() {
    const grid = $('#experiments-grid');
    grid.innerHTML = '<div class="loading-card">Loading experiments...</div>';
    
    try {
        const data = await api.getExperiments();
        const experiments = data.experiments || [];
        
        if (experiments.length === 0) {
            grid.innerHTML = '<div class="loading-card">No experiments yet. Create one to get started.</div>';
            return;
        }
        
        grid.innerHTML = experiments.map(exp => `
            <div class="experiment-card" data-id="${exp.id}">
                <div class="experiment-header">
                    <div class="experiment-info">
                        <h4>${exp.name}</h4>
                        <p>${exp.description || 'No description'}</p>
                    </div>
                    <span class="experiment-status ${exp.status}">${exp.status}</span>
                </div>
                <div class="experiment-body">
                    <div class="experiment-stats">
                        <div class="stat-item">
                            <div class="stat-value">${formatNumber(exp.control_count || 0)}</div>
                            <div class="stat-label">Control</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-value">${formatNumber(exp.test_count || 0)}</div>
                            <div class="stat-label">Test</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-value">${((exp.traffic_split || 0.5) * 100).toFixed(0)}%</div>
                            <div class="stat-label">Split</div>
                        </div>
                    </div>
                    <div class="experiment-actions">
                        ${exp.status === 'draft' ? `
                            <button class="btn-start" onclick="startExperiment('${exp.id}')">Start</button>
                        ` : exp.status === 'running' ? `
                            <button class="btn-pause" onclick="pauseExperiment('${exp.id}')">Pause</button>
                            <button class="btn-stop" onclick="stopExperiment('${exp.id}')">Stop</button>
                        ` : exp.status === 'paused' ? `
                            <button class="btn-start" onclick="startExperiment('${exp.id}')">Resume</button>
                            <button class="btn-stop" onclick="stopExperiment('${exp.id}')">Stop</button>
                        ` : `
                            <button class="btn-secondary" onclick="viewResults('${exp.id}')">View Results</button>
                        `}
                    </div>
                </div>
            </div>
        `).join('');
    } catch (error) {
        console.error('Failed to load experiments:', error);
        grid.innerHTML = `<div class="loading-card">${error.message}</div>`;
    }
}

// ============================================
// Experiment Actions
// ============================================

async function startExperiment(id) {
    try {
        await api.startExperiment(id);
        showToast('Experiment started', 'success');
        loadExperiments();
    } catch (error) {
        showToast(error.message, 'error');
    }
}

async function pauseExperiment(id) {
    try {
        await api.pauseExperiment(id);
        showToast('Experiment paused', 'info');
        loadExperiments();
    } catch (error) {
        showToast(error.message, 'error');
    }
}

async function stopExperiment(id) {
    try {
        await api.stopExperiment(id);
        showToast('Experiment stopped', 'info');
        loadExperiments();
    } catch (error) {
        showToast(error.message, 'error');
    }
}

async function viewResults(id) {
    try {
        const results = await api.getExperimentResults(id);
        // For now, just show a toast with summary
        showToast(`Control: ${results.control?.count || 0}, Test: ${results.test?.count || 0}`, 'info');
    } catch (error) {
        showToast(error.message, 'error');
    }
}

async function handleCreateExperiment(e) {
    e.preventDefault();
    
    const name = $('#exp-name').value;
    const description = $('#exp-description').value;
    const controlRules = $('#exp-control-rules').value.split(',').map(s => s.trim()).filter(Boolean);
    const testRules = $('#exp-test-rules').value.split(',').map(s => s.trim()).filter(Boolean);
    const trafficSplit = parseFloat($('#exp-split').value);
    
    try {
        await api.createExperiment({
            name,
            description,
            control_rules: controlRules,
            test_rules: testRules,
            traffic_split: trafficSplit,
        });
        
        closeModal();
        showToast('Experiment created', 'success');
        loadExperiments();
    } catch (error) {
        showToast(error.message, 'error');
    }
}

// ============================================
// Modal
// ============================================

function openModal() {
    $('#experiment-modal').classList.add('active');
    $('#experiment-form').reset();
}

function closeModal() {
    $('#experiment-modal').classList.remove('active');
}

// Transaction Modal
async function openTransactionModal() {
    $('#transaction-modal').classList.add('active');
    await loadAccountsDropdown();
}

async function loadAccountsDropdown() {
    const select = $('#tx-account-select');
    select.innerHTML = '<option value="">Loading accounts...</option>';
    
    try {
        const data = await api.getAccounts();
        const accounts = data.accounts || [];
        
        let options = '<option value="">-- Select an account --</option>';
        options += '<option value="__new__">➕ Create New Account</option>';
        
        if (accounts.length > 0) {
            options += '<optgroup label="Existing Accounts">';
            accounts.forEach(acc => {
                const label = `${acc.id.substring(0, 8)}... (${acc.account_type}, ${acc.risk_profile})`;
                options += `<option value="${acc.id}">${label}</option>`;
            });
            options += '</optgroup>';
        }
        
        select.innerHTML = options;
        
        // Auto-select first account if available
        if (accounts.length > 0) {
            select.value = accounts[0].id;
            $('#tx-account-id').value = accounts[0].id;
        }
    } catch (error) {
        console.error('Failed to load accounts:', error);
        select.innerHTML = '<option value="__new__">➕ Create New Account (no existing accounts)</option>';
    }
}

function onAccountSelect() {
    const select = $('#tx-account-select');
    const newAccountSection = $('#new-account-section');
    const accountIdInput = $('#tx-account-id');
    
    if (select.value === '__new__') {
        newAccountSection.style.display = 'block';
        accountIdInput.value = '';
    } else {
        newAccountSection.style.display = 'none';
        accountIdInput.value = select.value;
    }
}
window.onAccountSelect = onAccountSelect;

function closeTransactionModal() {
    $('#transaction-modal').classList.remove('active');
    $('#new-account-section').style.display = 'none';
}

async function handleCreateTransaction(e) {
    e.preventDefault();
    
    let accountId = $('#tx-account-id').value.trim();
    const accountSelect = $('#tx-account-select').value;
    const newAccountName = $('#tx-new-account-name').value.trim();
    
    const amount = parseFloat($('#tx-amount').value);
    const currency = $('#tx-currency').value;
    const merchant = $('#tx-merchant').value.trim();
    const category = $('#tx-category').value;
    const country = $('#tx-country').value;
    const channel = $('#tx-channel').value;
    const location = $('#tx-location').value.trim();
    
    // If creating new account
    if (accountSelect === '__new__') {
        if (!newAccountName) {
            showToast('Please enter a name for the new account', 'error');
            return;
        }
        
        try {
            showToast('Creating new account...', 'info');
            
            // First create a user
            const userResponse = await api.request('/auth/register', {
                method: 'POST',
                body: JSON.stringify({
                    email: `${newAccountName.toLowerCase().replace(/[^a-z0-9]/g, '')}${Date.now()}@demo.local`,
                    password: 'DemoPassword123!',
                    name: newAccountName,
                    role: 'user'
                })
            });
            
            // Then create account for that user
            const accountResponse = await api.createAccount(userResponse.user.id);
            accountId = accountResponse.id;
            
            showToast(`Account created: ${accountId.substring(0, 8)}...`, 'success');
        } catch (error) {
            showToast(`Failed to create account: ${error.message}`, 'error');
            return;
        }
    }
    
    if (!accountId) {
        showToast('Please select an account', 'error');
        return;
    }
    
    try {
        const result = await api.createTransaction({
            account_id: accountId,
            amount: amount,
            currency: currency,
            merchant: merchant,
            merchant_category: category,
            country: country,
            channel: channel,
            location: location,
            idempotency_key: `dashboard-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        });
        
        closeTransactionModal();
        showToast(`Transaction created: ${result.transaction_id?.substring(0, 8)}...`, 'success');
        
        // Reload transactions after a short delay (for worker to process)
        setTimeout(() => {
            loadRecentTransactions();
            loadRiskSummary();
            loadRiskDistribution();
        }, 2000);
    } catch (error) {
        showToast(error.message, 'error');
    }
}

// Make modal functions available globally
window.openTransactionModal = openTransactionModal;
window.closeTransactionModal = closeTransactionModal;

// ============================================
// Event Listeners
// ============================================

document.addEventListener('DOMContentLoaded', () => {
    // Check auth state
    if (state.token) {
        showDashboard();
    } else {
        showLoginScreen();
    }
    
    // Login form
    $('#login-form').addEventListener('submit', handleLogin);
    
    // Logout
    $('#logout-btn').addEventListener('click', logout);
    
    // Navigation
    $$('.nav-item').forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            switchView(item.dataset.view);
        });
    });
    
    // Refresh button
    $('#refresh-btn').addEventListener('click', () => {
        loadDashboardData();
        showToast('Refreshed', 'info');
    });
    
    // Flagged pagination
    $('#flagged-prev').addEventListener('click', () => {
        if (state.flaggedPage > 1) {
            state.flaggedPage--;
            loadFlaggedTransactions();
        }
    });
    
    $('#flagged-next').addEventListener('click', () => {
        state.flaggedPage++;
        loadFlaggedTransactions();
    });
    
    // Transaction search
    $('#search-tx-btn').addEventListener('click', searchTransactions);
    $('#tx-search').addEventListener('keypress', (e) => {
        if (e.key === 'Enter') searchTransactions();
    });
    $('#clear-search-btn').addEventListener('click', () => {
        $('#tx-search').value = '';
        const tableHeader = $('#transactions-view .table-header h3');
        if (tableHeader) tableHeader.textContent = 'Recent Transactions';
        loadRecentTransactions();
        showToast('Search cleared', 'info');
    });
    
    // New Transaction modal
    $('#new-tx-btn').addEventListener('click', openTransactionModal);
    $('#transaction-modal').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) closeTransactionModal();
    });
    $('#transaction-form').addEventListener('submit', handleCreateTransaction);
    
    // Experiment modal
    $('#new-experiment-btn').addEventListener('click', openModal);
    $$('.modal-close, .modal-cancel').forEach(btn => {
        btn.addEventListener('click', closeModal);
    });
    $('#experiment-modal').addEventListener('click', (e) => {
        if (e.target === e.currentTarget) closeModal();
    });
    $('#experiment-form').addEventListener('submit', handleCreateExperiment);
    
    // CSV Upload
    setupCsvUpload();
});

// Make functions available globally for onclick handlers
window.startExperiment = startExperiment;
window.pauseExperiment = pauseExperiment;
window.stopExperiment = stopExperiment;
window.viewResults = viewResults;

// ============================================
// CSV Upload Functionality
// ============================================

let csvData = [];

function toggleCsvUpload() {
    const section = $('#csv-upload-section');
    const content = $('#csv-upload-content');
    
    if (content.style.display === 'none') {
        content.style.display = 'block';
        section.classList.add('expanded');
    } else {
        content.style.display = 'none';
        section.classList.remove('expanded');
    }
}
window.toggleCsvUpload = toggleCsvUpload;

function setupCsvUpload() {
    const dropzone = $('#csv-dropzone');
    const fileInput = $('#csv-file');
    
    if (!dropzone || !fileInput) return;
    
    // Drag and drop handlers
    dropzone.addEventListener('dragover', (e) => {
        e.preventDefault();
        dropzone.classList.add('dragover');
    });
    
    dropzone.addEventListener('dragleave', () => {
        dropzone.classList.remove('dragover');
    });
    
    dropzone.addEventListener('drop', (e) => {
        e.preventDefault();
        dropzone.classList.remove('dragover');
        const file = e.dataTransfer.files[0];
        if (file && file.name.endsWith('.csv')) {
            handleCsvFile(file);
        } else {
            showToast('Please upload a CSV file', 'error');
        }
    });
    
    // Click to upload
    dropzone.addEventListener('click', () => fileInput.click());
    
    fileInput.addEventListener('change', (e) => {
        const file = e.target.files[0];
        if (file) handleCsvFile(file);
    });
}

function handleCsvFile(file) {
    const reader = new FileReader();
    
    reader.onload = (e) => {
        const text = e.target.result;
        const lines = text.trim().split('\n');
        
        if (lines.length < 2) {
            showToast('CSV must have header and at least one data row', 'error');
            return;
        }
        
        // Parse header
        const header = lines[0].split(',').map(h => h.trim().toLowerCase());
        const requiredFields = ['account_id', 'amount'];
        
        const hasRequired = requiredFields.every(f => header.includes(f));
        if (!hasRequired) {
            showToast('CSV must have account_id and amount columns', 'error');
            return;
        }
        
        // Parse data rows
        csvData = [];
        for (let i = 1; i < lines.length; i++) {
            const values = parseCSVLine(lines[i]);
            if (values.length === header.length) {
                const row = {};
                header.forEach((h, idx) => {
                    row[h] = values[idx];
                });
                csvData.push(row);
            }
        }
        
        if (csvData.length === 0) {
            showToast('No valid data rows found', 'error');
            return;
        }
        
        // Show preview
        showCsvPreview(file.name, header, csvData);
    };
    
    reader.readAsText(file);
}

function parseCSVLine(line) {
    const result = [];
    let current = '';
    let inQuotes = false;
    
    for (let i = 0; i < line.length; i++) {
        const char = line[i];
        if (char === '"') {
            inQuotes = !inQuotes;
        } else if (char === ',' && !inQuotes) {
            result.push(current.trim());
            current = '';
        } else {
            current += char;
        }
    }
    result.push(current.trim());
    return result;
}

function showCsvPreview(fileName, headers, data) {
    $('#csv-dropzone').style.display = 'none';
    $('#csv-preview').style.display = 'block';
    $('#csv-file-name').textContent = fileName;
    $('#csv-row-count').textContent = `${data.length} transactions`;
    
    // Build preview table (show first 5 rows)
    const previewData = data.slice(0, 5);
    const displayHeaders = ['account_id', 'amount', 'merchant', 'country'].filter(h => headers.includes(h));
    
    let tableHtml = '<table><thead><tr>';
    displayHeaders.forEach(h => {
        tableHtml += `<th>${h}</th>`;
    });
    tableHtml += '</tr></thead><tbody>';
    
    previewData.forEach(row => {
        tableHtml += '<tr>';
        displayHeaders.forEach(h => {
            const value = row[h] || '--';
            tableHtml += `<td>${value.substring(0, 20)}${value.length > 20 ? '...' : ''}</td>`;
        });
        tableHtml += '</tr>';
    });
    
    if (data.length > 5) {
        tableHtml += `<tr><td colspan="${displayHeaders.length}" style="text-align:center;color:var(--text-muted)">... and ${data.length - 5} more rows</td></tr>`;
    }
    
    tableHtml += '</tbody></table>';
    $('#csv-preview-table').innerHTML = tableHtml;
}

function clearCsvUpload() {
    csvData = [];
    $('#csv-dropzone').style.display = 'block';
    $('#csv-preview').style.display = 'none';
    $('#csv-progress').style.display = 'none';
    $('#csv-file').value = '';
}
window.clearCsvUpload = clearCsvUpload;

async function processCsvUpload() {
    if (csvData.length === 0) {
        showToast('No data to upload', 'error');
        return;
    }
    
    $('#csv-preview').style.display = 'none';
    $('#csv-progress').style.display = 'block';
    
    let processed = 0;
    let success = 0;
    let failed = 0;
    
    for (const row of csvData) {
        try {
            await api.createTransaction({
                account_id: row.account_id,
                amount: parseFloat(row.amount) || 100,
                currency: row.currency || 'USD',
                merchant: row.merchant || 'Bulk Import',
                merchant_category: row.category || row.merchant_category || 'retail',
                country: row.country || 'US',
                channel: row.channel || 'online',
                location: row.location || row.city || '',
                idempotency_key: `csv-${Date.now()}-${processed}-${Math.random().toString(36).substr(2, 6)}`,
            });
            success++;
        } catch (error) {
            console.error('Failed to create transaction:', row, error);
            failed++;
        }
        
        processed++;
        const progress = (processed / csvData.length) * 100;
        $('#csv-progress-fill').style.width = `${progress}%`;
        $('#csv-progress-text').textContent = `Processing ${processed}/${csvData.length}... (${success} success, ${failed} failed)`;
        
        // Small delay to avoid overwhelming the API
        await new Promise(r => setTimeout(r, 50));
    }
    
    showToast(`Uploaded ${success} transactions (${failed} failed)`, failed > 0 ? 'warning' : 'success');
    
    // Reset and reload
    setTimeout(() => {
        clearCsvUpload();
        loadRecentTransactions();
    }, 1500);
}
window.processCsvUpload = processCsvUpload;

function downloadCsvTemplate(event) {
    event.preventDefault();
    
    const template = `account_id,amount,currency,merchant,category,country,channel,location
37327978-36d6-4713-b425-6194ffff97ad,150.00,USD,Amazon,retail,US,online,Seattle
37327978-36d6-4713-b425-6194ffff97ad,25000.00,USD,Suspicious Vendor,crypto,RU,wire,Moscow
37327978-36d6-4713-b425-6194ffff97ad,500.00,EUR,Cafe Paris,food,FR,pos,Paris
37327978-36d6-4713-b425-6194ffff97ad,10000.00,USD,Shell Company,gambling,NK,online,Pyongyang`;
    
    const blob = new Blob([template], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'transaction_template.csv';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    
    showToast('Template downloaded', 'success');
}
window.downloadCsvTemplate = downloadCsvTemplate;
