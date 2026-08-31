/** Authenticated teaching UI for orders, margin, risk and read-only AI evidence. */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  adjustMargin,
  createConditional,
  faucet,
  getBalances,
  getFunding,
  getInsurance,
  getLiquidations,
  getOrders,
  getPositions,
  getRisk,
  login,
  me,
  placeOrder,
  register,
  setLeverage,
  setMarginMode,
} from './api'

type Props = { symbol: string; markPrice: string }

const panelClass =
  'rounded-[10px] border border-[#242a32] bg-[rgba(17,21,27,0.94)] p-[18px] shadow-[0_18px_46px_rgba(0,0,0,0.22)]'
const panelTitleClass = 'flex items-baseline justify-between border-b border-[#242a32] pb-[14px]'
const panelHeadingClass = 'm-0 text-[15px] font-semibold'
const panelCaptionClass = 'text-[11px] text-[#697480]'
const buttonBaseClass = 'cursor-pointer rounded-[6px] border border-[#303843] px-2.5 py-2'
const neutralButtonClass = `${buttonBaseClass} bg-[#1b2129] text-[#c7cdd5]`
const primaryButtonClass = `${buttonBaseClass} bg-[#2ebd85] text-[#08110d]`
const inputClass = 'w-full rounded-[6px] border border-[#303843] bg-[#0b0e11] px-2.5 py-[9px] text-[#eaecef]'
const labelClass = 'my-2.5 grid gap-[5px] text-xs text-[#848e9c]'
const buttonRowClass = 'mt-3 flex flex-wrap gap-[7px]'

/** Renders the simulated contract workflow without becoming a source of truth. */
export function TradingPanel({ symbol, markPrice }: Props) {
  const queryClient = useQueryClient()
  const [email, setEmail] = useState('demo@example.com')
  const [password, setPassword] = useState('ExchangeLab!2026')
  const [side, setSide] = useState<'buy' | 'sell'>('buy')
  const [price, setPrice] = useState('50000.00')
  const [quantity, setQuantity] = useState('0.010')
  const [leverage, setLeverageValue] = useState(10)
  const [message, setMessage] = useState('')
  const [aiText, setAIText] = useState('')
  const session = useQuery({ queryKey: ['me'], queryFn: ({ signal }) => me(signal), retry: false })
  const enabled = !!session.data
  const balances = useQuery({
    queryKey: ['balances'],
    queryFn: ({ signal }) => getBalances(signal),
    enabled,
    refetchInterval: 2000,
  })
  const positions = useQuery({
    queryKey: ['positions'],
    queryFn: ({ signal }) => getPositions(signal),
    enabled,
    refetchInterval: 1000,
  })
  const risk = useQuery({
    queryKey: ['risk', symbol],
    queryFn: ({ signal }) => getRisk(symbol, signal),
    enabled,
    refetchInterval: 1000,
  })
  const orders = useQuery({
    queryKey: ['orders'],
    queryFn: ({ signal }) => getOrders(signal),
    enabled,
    refetchInterval: 1000,
  })
  useQuery({ queryKey: ['funding'], queryFn: ({ signal }) => getFunding(signal), enabled, refetchInterval: 5000 })
  useQuery({
    queryKey: ['liquidations'],
    queryFn: ({ signal }) => getLiquidations(signal),
    enabled,
    refetchInterval: 5000,
  })
  useQuery({ queryKey: ['insurance'], queryFn: ({ signal }) => getInsurance(signal), refetchInterval: 5000 })
  const refresh = () => queryClient.invalidateQueries()
  const action = useMutation({
    mutationFn: async (fn: () => Promise<unknown>) => fn(),
    onSuccess: () => {
      setMessage('操作成功')
      refresh()
    },
    onError: () => setMessage('操作失败，请检查输入或余额'),
  })

  if (!enabled)
    return (
      <section className={`${panelClass} mt-3`}>
        <h2 className="m-0 text-lg font-semibold">模拟账户</h2>
        <p>注册或登录后可完成永续合约全流程。</p>
        <div className="grid grid-cols-1 gap-2 min-[621px]:grid-cols-[2fr_2fr_1fr_1fr]">
          <input
            className={inputClass}
            aria-label="邮箱"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
          <input
            className={inputClass}
            aria-label="密码"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => login(email, password))}
          >
            登录
          </button>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => register(email, password))}
          >
            注册
          </button>
        </div>
        <p className="min-h-[18px] text-xs text-[#f0b90b]">{message}</p>
      </section>
    )

  const current = positions.data?.find((position) => position.symbol === symbol)
  const riskMetrics = [
    ['仓位', current?.size ?? '0.000'],
    ['开仓均价', current?.entry_price ?? '—'],
    ['标记价格', risk.data?.mark_price ?? markPrice],
    ['未实现盈亏', risk.data?.unrealized_pnl ?? '0.00000000'],
    ['保证金余额', risk.data?.margin_balance ?? '0.00000000'],
    ['保证金率', `${risk.data?.margin_ratio_bps ?? 0} bps`],
    ['距强平', `${risk.data?.liquidation_distance_bps ?? 0} bps`],
  ]
  return (
    <section className="mt-3 grid grid-cols-1 gap-3 min-[621px]:grid-cols-2 min-[981px]:grid-cols-[1.1fr_1.6fr_1fr_1fr]">
      <div className={panelClass}>
        <div className={panelTitleClass}>
          <h2 className={panelHeadingClass}>合约下单</h2>
          <span className={panelCaptionClass}>{session.data.email}</span>
        </div>
        <div className={buttonRowClass}>
          <button
            type="button"
            className={`${buttonBaseClass} ${side === 'buy' ? 'bg-[#2ebd85] text-[#08110d]' : 'bg-[#1b2129] text-[#c7cdd5]'}`}
            onClick={() => setSide('buy')}
          >
            开多 / 买入
          </button>
          <button
            type="button"
            className={`${buttonBaseClass} ${side === 'sell' ? 'bg-[#f6465d] text-white' : 'bg-[#1b2129] text-[#c7cdd5]'}`}
            onClick={() => setSide('sell')}
          >
            开空 / 卖出
          </button>
        </div>
        <label className={labelClass}>
          价格
          <input className={inputClass} value={price} onChange={(event) => setPrice(event.target.value)} />
        </label>
        <label className={labelClass}>
          数量
          <input className={inputClass} value={quantity} onChange={(event) => setQuantity(event.target.value)} />
        </label>
        <label className={labelClass}>
          杠杆
          <input
            className={inputClass}
            type="number"
            min="1"
            max="50"
            value={leverage}
            onChange={(event) => setLeverageValue(Number(event.target.value))}
          />
        </label>
        <div className={buttonRowClass}>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => setLeverage(symbol, leverage))}
          >
            设置杠杆
          </button>
          <button
            className={primaryButtonClass}
            type="button"
            onClick={() => action.mutate(() => placeOrder({ symbol, side, price, quantity, leverage }))}
          >
            提交限价单
          </button>
        </div>
      </div>
      <div className={panelClass} data-testid="risk-card">
        <div className={panelTitleClass}>
          <h2 className={panelHeadingClass}>仓位与风险</h2>
          <span className={`text-[11px] ${risk.data?.fresh ? 'text-[#2ebd85]' : 'text-[#f6465d]'}`}>
            {risk.data?.fresh ? '价格新鲜' : '价格陈旧'}
          </span>
        </div>
        <div className="mt-[14px] grid grid-cols-2 gap-2.5">
          {riskMetrics.map(([label, value]) => (
            <div className="grid gap-1 rounded-[6px] bg-[#0c1015] p-[9px]" key={label}>
              <span className="text-[11px] text-[#7f8997]">{label}</span>
              <strong className="[font-variant-numeric:tabular-nums]">{value}</strong>
            </div>
          ))}
        </div>
        <div className={buttonRowClass}>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => setMarginMode('isolated'))}
          >
            逐仓
          </button>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => setMarginMode('cross'))}
          >
            全仓
          </button>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => adjustMargin(symbol, '10.00000000'))}
          >
            追加 10 USDT
          </button>
        </div>
      </div>
      <div className={panelClass}>
        <div className={panelTitleClass}>
          <h2 className={panelHeadingClass}>资金与订单</h2>
          <span className={panelCaptionClass}>后端真值</span>
        </div>
        <p>
          可用：
          <strong className="[font-variant-numeric:tabular-nums]">
            {balances.data?.[0]?.available ?? '0.00000000'} USDT
          </strong>
        </p>
        <p>
          锁定：
          <strong className="[font-variant-numeric:tabular-nums]">
            {balances.data?.[0]?.locked ?? '0.00000000'} USDT
          </strong>
        </p>
        <div className={buttonRowClass}>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => action.mutate(() => faucet('10000.00000000'))}
          >
            领取 10,000 模拟 USDT
          </button>
        </div>
        <div className="mt-3">
          {orders.data?.slice(0, 4).map((order) => (
            <div className="flex justify-between border-t border-[#242a32] py-[7px] text-xs" key={order.id}>
              <span>
                {order.side} {order.quantity}
              </span>
              <strong>{order.status}</strong>
            </div>
          ))}
        </div>
      </div>
      <div className={panelClass}>
        <div className={panelTitleClass}>
          <h2 className={panelHeadingClass}>止盈止损 / AI</h2>
          <span className={panelCaptionClass}>只读解释</span>
        </div>
        <p>基于标记价格创建教学级止损，触发后不会反向扩大仓位。</p>
        <div className={buttonRowClass}>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() =>
              action.mutate(() =>
                createConditional({
                  symbol,
                  type: 'stop_loss',
                  trigger_source: 'mark',
                  trigger_direction: side === 'buy' ? 'lte' : 'gte',
                  trigger_price: price,
                  quantity,
                }),
              )
            }
          >
            以 {price} 创建止损
          </button>
          <button
            className={neutralButtonClass}
            type="button"
            onClick={() => {
              setAIText('生成中…')
              const stream = new EventSource(`/v1/ai/contract-risk-summary?symbol=${encodeURIComponent(symbol)}`)
              stream.addEventListener('risk-summary', (event) => {
                const parsed = JSON.parse((event as MessageEvent).data)
                setAIText(parsed.data.summary.risks.join('；'))
                stream.close()
              })
              stream.onerror = () => {
                setAIText('AI 暂不可用，交易核心不受影响')
                stream.close()
              }
            }}
          >
            AI 风险解释
          </button>
        </div>
        <p>{aiText}</p>
        <p className="min-h-[18px] text-xs text-[#f0b90b]">{message}</p>
      </div>
    </section>
  )
}
