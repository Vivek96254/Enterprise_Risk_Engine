/**
 * Enterprise Risk Engine - k6 Load Test Script
 * 
 * Usage:
 *   k6 run scripts/load_test.js
 *   k6 run --vus 50 --duration 5m scripts/load_test.js
 *   k6 run --env BASE_URL=https://your-api.onrender.com scripts/load_test.js
 * 
 * Scenarios:
 *   - smoke: Quick validation (5 VUs, 30s)
 *   - load: Normal load (50 VUs, 5m)
 *   - stress: High load (100 VUs, 10m)
 *   - spike: Sudden spike (10→200→10 VUs)
 */

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

// Custom metrics
const transactionCounter = new Counter('transactions_created');
const riskScoreRate = new Rate('risk_score_success');
const transactionDuration = new Trend('transaction_duration');

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const SCENARIO = __ENV.SCENARIO || 'smoke';

// Test scenarios
export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 5,
      duration: '30s',
      exec: 'mainTest',
      tags: { scenario: 'smoke' },
    },
    load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '1m', target: 50 },   // Ramp up
        { duration: '3m', target: 50 },   // Stay at 50
        { duration: '1m', target: 0 },    // Ramp down
      ],
      exec: 'mainTest',
      tags: { scenario: 'load' },
      startTime: '35s', // Start after smoke
    },
    stress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 100 },  // Ramp up
        { duration: '5m', target: 100 },  // Stay at 100
        { duration: '2m', target: 200 },  // Push to 200
        { duration: '1m', target: 0 },    // Ramp down
      ],
      exec: 'mainTest',
      tags: { scenario: 'stress' },
      startTime: '6m', // Start after load
    },
    spike: {
      executor: 'ramping-vus',
      startVUs: 10,
      stages: [
        { duration: '10s', target: 10 },   // Baseline
        { duration: '10s', target: 200 },  // Spike!
        { duration: '30s', target: 200 },  // Stay high
        { duration: '10s', target: 10 },   // Back down
        { duration: '30s', target: 10 },   // Recovery
      ],
      exec: 'mainTest',
      tags: { scenario: 'spike' },
      startTime: '16m', // Start after stress
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'],
    transactions_created: ['count>100'],
    risk_score_success: ['rate>0.95'],
    transaction_duration: ['p(95)<300'],
  },
};

// Shared state
let authToken = null;
let accountId = null;
let userId = null;

// Setup: Run once before all VUs
export function setup() {
  console.log(`🚀 Starting load test against ${BASE_URL}`);
  
  // Register a test user
  const email = `loadtest_${randomString(8)}@test.com`;
  const password = 'LoadTest123!';
  
  const registerRes = http.post(`${BASE_URL}/api/v1/auth/register`, JSON.stringify({
    email: email,
    password: password,
    role: 'admin',
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  if (registerRes.status !== 201) {
    // User might exist, try login
    const loginRes = http.post(`${BASE_URL}/api/v1/auth/login`, JSON.stringify({
      email: 'loadtest@test.com',
      password: 'LoadTest123!',
    }), {
      headers: { 'Content-Type': 'application/json' },
    });

    if (loginRes.status !== 200) {
      console.error('Failed to authenticate:', loginRes.body);
      return null;
    }

    const loginData = JSON.parse(loginRes.body);
    authToken = loginData.token;
    userId = loginData.user.id;
  } else {
    const registerData = JSON.parse(registerRes.body);
    authToken = registerData.token;
    userId = registerData.user.id;
  }

  // Create an account for testing
  const accountRes = http.post(`${BASE_URL}/api/v1/accounts`, JSON.stringify({
    user_id: userId,
    account_type: 'standard',
  }), {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${authToken}`,
    },
  });

  if (accountRes.status === 201) {
    accountId = JSON.parse(accountRes.body).id;
  }

  console.log(`✅ Setup complete. User: ${userId}, Account: ${accountId}`);
  
  return { authToken, accountId, userId };
}

// Main test function
export function mainTest(data) {
  if (!data || !data.authToken) {
    console.error('No auth token available');
    return;
  }

  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${data.authToken}`,
  };

  group('Transaction Ingestion', () => {
    // Single transaction ingestion
    const txPayload = generateTransaction(data.accountId);
    const startTime = Date.now();
    
    const res = http.post(`${BASE_URL}/api/v1/transactions`, JSON.stringify(txPayload), { headers });
    
    const duration = Date.now() - startTime;
    transactionDuration.add(duration);

    const success = check(res, {
      'transaction created': (r) => r.status === 201,
      'has transaction_id': (r) => JSON.parse(r.body).transaction_id !== undefined,
    });

    if (success) {
      transactionCounter.add(1);
    }

    sleep(0.1);
  });

  group('Batch Ingestion', () => {
    // Batch transaction ingestion (10 transactions)
    const transactions = [];
    for (let i = 0; i < 10; i++) {
      transactions.push(generateTransaction(data.accountId));
    }

    const res = http.post(`${BASE_URL}/api/v1/transactions/batch`, JSON.stringify({
      transactions: transactions,
    }), { headers });

    check(res, {
      'batch accepted': (r) => r.status === 200,
      'all successful': (r) => JSON.parse(r.body).successful === 10,
    });

    if (res.status === 200) {
      const result = JSON.parse(res.body);
      transactionCounter.add(result.successful);
    }

    sleep(0.2);
  });

  group('Analytics Queries', () => {
    // Risk summary
    const summaryRes = http.get(`${BASE_URL}/api/v1/risk/summary`, { headers });
    check(summaryRes, {
      'summary returned': (r) => r.status === 200,
    });

    // Risk distribution
    const distRes = http.get(`${BASE_URL}/api/v1/risk/distribution?days=7`, { headers });
    check(distRes, {
      'distribution returned': (r) => r.status === 200,
    });

    // Flagged transactions
    const flaggedRes = http.get(`${BASE_URL}/api/v1/transactions/flagged?page=1&page_size=20`, { headers });
    check(flaggedRes, {
      'flagged returned': (r) => r.status === 200,
    });

    riskScoreRate.add(summaryRes.status === 200 && distRes.status === 200);

    sleep(0.1);
  });

  group('Account Operations', () => {
    // Get account risk profile
    if (data.accountId) {
      const profileRes = http.get(`${BASE_URL}/api/v1/risk/account/${data.accountId}`, { headers });
      check(profileRes, {
        'profile returned': (r) => r.status === 200 || r.status === 404,
      });
    }

    sleep(0.1);
  });

  group('System Metrics', () => {
    const metricsRes = http.get(`${BASE_URL}/api/v1/metrics/system`, { headers });
    check(metricsRes, {
      'metrics returned': (r) => r.status === 200,
    });

    sleep(0.1);
  });

  // Small delay between iterations
  sleep(randomIntBetween(0.1, 0.5));
}

// Helper: Generate random transaction
function generateTransaction(accountId) {
  const merchants = ['Amazon', 'Walmart', 'Target', 'Starbucks', 'Apple', 'Netflix', 'Uber', 'Gas Station'];
  const locations = ['New York, NY', 'Los Angeles, CA', 'Chicago, IL', 'Houston, TX', 'Seattle, WA', 'Miami, FL'];
  const countries = ['US', 'US', 'US', 'CA', 'GB', 'DE', 'IR']; // Include high-risk country
  const channels = ['online', 'pos', 'atm'];

  // Generate realistic amounts with occasional high amounts
  let amount;
  const rand = Math.random();
  if (rand < 0.7) {
    amount = randomIntBetween(10, 500);        // Normal: $10-500
  } else if (rand < 0.9) {
    amount = randomIntBetween(500, 2000);      // Medium: $500-2000
  } else if (rand < 0.98) {
    amount = randomIntBetween(2000, 10000);    // High: $2000-10000
  } else {
    amount = randomIntBetween(10000, 50000);   // Critical: $10000+
  }

  return {
    account_id: accountId,
    amount: amount,
    currency: 'USD',
    merchant: merchants[randomIntBetween(0, merchants.length - 1)],
    merchant_category: 'retail',
    location: locations[randomIntBetween(0, locations.length - 1)],
    country: countries[randomIntBetween(0, countries.length - 1)],
    channel: channels[randomIntBetween(0, channels.length - 1)],
    idempotency_key: `k6-${__VU}-${__ITER}-${Date.now()}-${randomString(8)}`,
  };
}

// Teardown: Run once after all VUs complete
export function teardown(data) {
  console.log('🏁 Load test completed');
  console.log(`📊 Results available in k6 output`);
}

// Handle summary output
export function handleSummary(data) {
  return {
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
    'scripts/load_test_results.json': JSON.stringify(data, null, 2),
  };
}

// Simple text summary (k6 built-in not available, so we create our own)
function textSummary(data, options) {
  const metrics = data.metrics;
  let output = '\n📊 Load Test Summary\n';
  output += '═'.repeat(50) + '\n\n';

  if (metrics.http_req_duration) {
    output += `⏱️  HTTP Request Duration:\n`;
    output += `   avg: ${metrics.http_req_duration.values.avg.toFixed(2)}ms\n`;
    output += `   p95: ${metrics.http_req_duration.values['p(95)'].toFixed(2)}ms\n`;
    output += `   p99: ${metrics.http_req_duration.values['p(99)'].toFixed(2)}ms\n\n`;
  }

  if (metrics.http_reqs) {
    output += `📨 Total Requests: ${metrics.http_reqs.values.count}\n`;
    output += `   Rate: ${metrics.http_reqs.values.rate.toFixed(2)}/s\n\n`;
  }

  if (metrics.http_req_failed) {
    output += `❌ Failed Requests: ${(metrics.http_req_failed.values.rate * 100).toFixed(2)}%\n\n`;
  }

  if (metrics.transactions_created) {
    output += `✅ Transactions Created: ${metrics.transactions_created.values.count}\n\n`;
  }

  return output;
}
