// TPT Credit Testnet Faucet - Web-based faucet server.
// Distributes testnet ucredit tokens to verified addresses.
// Rate limited: 1 request per IP per 24 hours.

require('dotenv').config();
const express = require('express');
const rateLimit = require('express-rate-limit');
const cors = require('cors');
const path = require('path');
const { StargateClient, assertIsDeliverTxSuccess } = require('@cosmjs/stargate');
const { DirectSecp256k1HdWallet } = require('@cosmjs/proto-signing');
const { GasPrice } = require('@cosmjs/stargate');
const { stringToPath } = require('@cosmjs/crypto');
const { fromHex, toHex } = require('@cosmjs/encoding');

const app = express();
const PORT = process.env.PORT || 3000;

// Configuration
const CHAIN_RPC_URL = process.env.CHAIN_RPC_URL || 'http://localhost:26657';
const CHAIN_API_URL = process.env.CHAIN_API_URL || 'http://localhost:1317';
const FAUCET_MNEMONIC = process.env.FAUCET_MNEMONIC;
const FAUCET_AMOUNT = process.env.FAUCET_AMOUNT || '10000000'; // 10 credits in ucredit
const CHAIN_ID = process.env.CHAIN_ID || 'tpt-credit-testnet-1';
const DENOM = 'ucredit';
const RATE_LIMIT_WINDOW_MS = parseInt(process.env.RATE_LIMIT_PER_IP || '86400000'); // 24 hours

// In-memory rate limit store (IP -> timestamp of last request)
const rateLimitStore = new Map();

// Middleware
app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
app.set('view engine', 'ejs');
app.set('views', path.join(__dirname, 'views'));
app.use(express.static(path.join(__dirname, 'public')));

// Validate faucet mnemonic on startup
if (!FAUCET_MNEMONIC) {
  console.error('ERROR: FAUCET_MNEMONIC environment variable is required');
  console.error('Generate one with: tptcreditd keys add faucet --keyring-backend test');
  process.exit(1);
}

// Initialize wallet
let faucetWallet;
let faucetAddress;

async function initFaucet() {
  try {
    faucetWallet = await DirectSecp256k1HdWallet.fromMnemonic(FAUCET_MNEMONIC, {
      prefix: 'cosmos',
    });
    const accounts = await faucetWallet.getAccounts();
    faucetAddress = accounts[0].address;
    console.log(`Faucet initialized with address: ${faucetAddress}`);
    console.log(`Faucet amount per request: ${FAUCET_AMOUNT} ucredit (${parseInt(FAUCET_AMOUNT) / 1000000} credits)`);
    console.log(`Chain RPC: ${CHAIN_RPC_URL}`);
    console.log(`Chain API: ${CHAIN_API_URL}`);
    console.log(`Chain ID: ${CHAIN_ID}`);
  } catch (err) {
    console.error('Failed to initialize faucet wallet:', err.message);
    process.exit(1);
  }
}

// Per-IP rate limiting
function getRateLimitMiddleware() {
  return rateLimit({
    windowMs: RATE_LIMIT_WINDOW_MS,
    max: 1,
    message: {
      error: 'Rate limit exceeded',
      message: 'You can only request tokens once per 24 hours. Please try again later.',
      retryAfter: `${Math.ceil(RATE_LIMIT_WINDOW_MS / 1000 / 60 / 60)} hours`,
    },
    standardHeaders: true,
    legacyHeaders: false,
    keyGenerator: (req) => {
      return req.ip || req.connection.remoteAddress;
    },
  });
}

// Routes
app.get('/', (req, res) => {
  res.render('index', {
    faucetAddress,
    amount: parseInt(FAUCET_AMOUNT) / 1000000,
    chainId: CHAIN_ID,
    error: null,
    success: null,
    txHash: null,
  });
});

app.post('/api/claim', getRateLimitMiddleware(), async (req, res) => {
  const { address } = req.body;

  // Validate address format (Cosmos bech32)
  if (!address || !address.startsWith('cosmos1') || address.length < 40 || address.length > 60) {
    return res.status(400).json({
      error: 'Invalid address',
      message: 'Please provide a valid Cosmos address starting with "cosmos1".',
    });
  }

  try {
    // Check if address is already registered on chain
    const accountResp = await fetch(`${CHAIN_API_URL}/cosmos/auth/v1beta1/accounts/${address}`);
    if (!accountResp.ok) {
      return res.status(400).json({
        error: 'Address not found',
        message: 'This address is not registered on the TPT Credit chain. Register first via the identity bridge.',
      });
    }

    // Connect to chain and send tokens
    const client = await StargateClient.connect(CHAIN_RPC_URL);
    const balance = await client.getBalance(address, DENOM);

    // Get sender account for sequence
    const sender = await client.getAccount(faucetAddress);
    if (!sender) {
      return res.status(500).json({
        error: 'Faucet error',
        message: 'Faucet account not found on chain. Please contact testnet administrators.',
      });
    }

    // Build and broadcast transfer
    const amount = [
      {
        denom: DENOM,
        amount: FAUCET_AMOUNT,
      },
    ];

    const fee = {
      amount: [],
      gas: '200000',
    };

    const result = await client.sendTokens(
      faucetAddress,
      address,
      amount,
      fee,
      'TPT Credit testnet faucet'
    );

    assertIsDeliverTxSuccess(result);

    const txHash = result.transactionHash || toHex(result.transactionHash);

    console.log(`Faucet dispatched ${FAUCET_AMOUNT} ucredit to ${address} (tx: ${txHash})`);

    res.json({
      success: true,
      message: `Successfully sent ${parseInt(FAUCET_AMOUNT) / 1000000} credits to ${address}`,
      txHash,
      amount: FAUCET_AMOUNT,
      address,
    });
  } catch (err) {
    console.error(`Faucet error for ${address}:`, err.message);
    res.status(500).json({
      error: 'Transaction failed',
      message: err.message,
    });
  }
});

// Health check endpoint
app.get('/health', (req, res) => {
  res.json({
    status: 'ok',
    faucetAddress,
    chainId: CHAIN_ID,
    uptime: process.uptime(),
  });
});

// Start server
initFaucet().then(() => {
  app.listen(PORT, '0.0.0.0', () => {
    console.log(`TPT Credit Faucet running on http://0.0.0.0:${PORT}`);
    console.log(`Faucet address: ${faucetAddress}`);
  });
});