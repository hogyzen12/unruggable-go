import express from 'express';
import fetch from 'node-fetch';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const app = express();
const port = 3000;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const tokenListPath = path.join(__dirname, 'strict_token_list.json');

const TIP_PAYMENT_ACCOUNTS = [
  "96gYZGLnJYVFmbjzopPSU6QiEV5fGqZNyN9nmNhvrZU5",
  "HFqU5x63VTqvQss8hp11i4wVV8bD44PvwucfZ2bU7gRe",
  "Cw8CFyM9FkoMi7K7Crf6HNQqf4uEMzpKw6QNghXLvLkY",
  "ADaUMid9yfUytqMBgopwjb2DTLSokTSzL1zt6iGPaS49",
  "DfXygSm4jCyNCybVYYK6DwvWqjKee8pbDmJGcLWNDXjh",
  "ADuUkR4vqLUMWXxW9gh6D6L8pMSawimctcNZ5pGwDcEt",
  "DttWaMuVvTiduZRnguLF7jNxTgiMBZ1hyAumKUiL2KRL",
  "3AVi9Tg9Uo68tJfuvoKvqKNWKkC5wPdSSdeBnizKZ6jT"
];

// API endpoint for querying data
app.get('/api/solana/:publicKey', async (req, res) => {
  try {
    const publicKey = req.params.publicKey;
    const solanaUrl = `https://mainnet.helius-rpc.com/?api-key=2c0388dc-a082-4cc5-bad9-29437f3c0715`;

    // Get the token list (either from cache or fetch new)
    let tokenList;
    if (fs.existsSync(tokenListPath)) {
      tokenList = JSON.parse(fs.readFileSync(tokenListPath, 'utf8'));
    } else {
      tokenList = await fetchTokenList();
      fs.writeFileSync(tokenListPath, JSON.stringify(tokenList, null, 2));
    }

    // Get assets by owner
    const assetsData = await getAssetsByOwner(solanaUrl, publicKey);

    // Extract native balance and assets
    const solBalanceLamports = assetsData.nativeBalance.lamports || 0;
    const solBalance = parseFloat((solBalanceLamports / 1e9).toFixed(3));
    const solPriceUSD = parseFloat((assetsData.nativeBalance.price_per_sol || 0).toFixed(3));
    const assets = assetsData.items;

    // Output raw response for troubleshooting
    //console.log('Assets Data:', JSON.stringify(assetsData, null, 2));

    // Calculate USD balance for each asset and filter out unwanted assets
    const assetsWithUSD = assets
      .map(asset => {
        const usdPrice = asset.token_info?.price_info?.price_per_token || 0;
        const balance = asset.token_info?.balance / Math.pow(10, asset.token_info?.decimals || 0);
        const tokenInfo = tokenList.find(token => token.address === asset.id);
        return {
          symbol: asset.token_info?.symbol || 'UNKNOWN',
          address: asset.id,
          balance: parseFloat(balance.toFixed(3)),
          usdPrice: parseFloat(usdPrice.toFixed(3)),
          usdBalance: parseFloat((balance * usdPrice).toFixed(3)),
          logoURI: tokenInfo?.logoURI || null,
        };
      })
      .filter(asset => asset.symbol !== 'UNKNOWN' && asset.usdBalance >= 0.01 && isTokenInList(asset.address, tokenList));

    // Output calculated values for troubleshooting
    //console.log('SOL Balance:', solBalance);
    //console.log('SOL Price USD:', solPriceUSD);
    //console.log('Assets with USD:', JSON.stringify(assetsWithUSD, null, 2));

    const result = {
      solBalance,
      solBalanceUSD: parseFloat((solBalance * solPriceUSD).toFixed(3)),
      assets: assetsWithUSD,
    };

    res.json(result);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// API endpoint for fetching transaction history
app.get('/api/solana/:publicKey/transactions', async (req, res) => {
  try {
    const publicKey = req.params.publicKey;
    const apiKey = '2c0388dc-a082-4cc5-bad9-29437f3c0715';
    const transactionsUrl = `https://api.helius.xyz/v0/addresses/${publicKey}/transactions?api-key=${apiKey}`;

    // Fetch transaction history
    const response = await fetch(transactionsUrl, {
      method: 'GET',
      headers: {
        'accept': 'application/json',
      },
    });
    const transactions = await response.json();

    // Output raw transaction data for troubleshooting
    //console.log('Raw Transaction Data:', JSON.stringify(transactions, null, 2));

    // Filter and format transactions
    const filteredTransactions = transactions
      .filter(tx => tx.feePayer === publicKey)
      .map(tx => {
        let isTipPayment = false;
        if (tx.nativeTransfers) {
          isTipPayment = tx.nativeTransfers.some(transfer => TIP_PAYMENT_ACCOUNTS.includes(transfer.toUserAccount));
        }
        return {
          time: new Date(tx.timestamp * 1000).toLocaleString(),
          description: isTipPayment ? 'JITO BUNDLE TIP' : (tx.description || 'Transaction on Solana'),
          type: tx.type || 'unknown',
          signature: tx.signature,
        };
      });

    // Output filtered transaction data for troubleshooting
    //console.log('Filtered Transaction Data:', JSON.stringify(filteredTransactions, null, 2));

    res.json(filteredTransactions);
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Function to get assets by owner
const getAssetsByOwner = async (url, publicKey) => {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      jsonrpc: '2.0',
      id: 'my-id',
      method: 'getAssetsByOwner',
      params: {
        ownerAddress: publicKey,
        page: 1,
        limit: 1000,
        displayOptions: {
          showFungible: true,
          showNativeBalance: true,
        },
      },
    }),
  });
  const { result } = await response.json();
  return result;
};

// Function to fetch the token list
const fetchTokenList = async () => {
  const response = await fetch('https://api.jup.ag/tokens/v1?tags=strict', {
    method: 'GET',
    headers: {
      'accept': 'application/json',
    },
  });
  return await response.json();
};

// Function to check if a token is in the list
const isTokenInList = (address, tokenList) => {
  return tokenList.some(token => token.address === address);
};

app.listen(port, () => {
  console.log(`Server running on http://localhost:${port}`);
});