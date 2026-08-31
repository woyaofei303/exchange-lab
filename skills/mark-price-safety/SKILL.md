---
name: mark-price-safety
description: Implement or review exchange-lab index price, mark price, source quality, freshness, premium clamp, PnL, margin, or liquidation price dependencies. Use whenever a risk decision consumes market prices.
---

# Mark Price Safety

1. Write a failing test for a stale, missing, future, duplicated, or outlier source.
2. Keep prices as integer atoms plus scale and use overflow-safe intermediates.
3. Require at least three fresh sources; use median filtering before weighted aggregation.
4. Record included and excluded source names, observation time, freshness, and formula version.
5. Clamp premium/basis explicitly before deriving mark price.
6. Make position PnL, margin, and liquidation depend on mark price, never a single last trade.
7. Fail closed when minimum source quality is not met; do not silently substitute last price.
8. Verify WebSocket/API fields retain exact decimal strings and timestamps.

AI may propose cases or explain evidence. It must not invent prices or trigger a liquidation.
