// Six decimal places, integer arithmetic, and no conversion between currencies.
const scale=1000000n
function units(value){
 const s=String(value??'')
 if(!/^\d{1,12}(\.\d{1,6})?$/.test(s))return null
 const [whole,fraction='']=s.split('.')
 return BigInt(whole)*scale+BigInt(fraction.padEnd(6,'0'))
}
function format(value){
 const n=value<0n?-value:value
 return (value<0n?'-':'')+String(n/scale)+'.'+String(n%scale).padStart(6,'0').replace(/0+$/,'').padEnd(2,'0')
}
const currencyOK=value=>/^[A-Z]{3}$/.test(value||'')
export function accountAmounts(row){
 const charge=row?.charge
 const paid=charge&&['1','4'].includes(charge.type)&&currencyOK(charge.card_currency)?units(charge.card_amount):null
 const totals=new Map(),seen=new Set()
 let pending=0,invalid=false
 for(const t of row?.returns||[]){
  if(!['2','5','16'].includes(t.trade?.tradeType))continue
  if(!t.confirmed||t.trade?.tradeStatus!=='2'){pending++;continue}
  if(t.id&&seen.has(t.id))continue
  if(t.id)seen.add(t.id)
  const n=units(t.confirmed_amount),currency=t.confirmed_currency
  if(n===null||!currencyOK(currency)){invalid=true;continue}
  totals.set(currency,(totals.get(currency)||0n)+n)
 }
 const returned=[...totals].sort(([a],[b])=>a.localeCompare(b)).map(([currency,n])=>({amount:format(n),currency}))
 const comparable=paid!==null&&charge.status==='2'&&!invalid&&[...totals.keys()].every(c=>c===charge.card_currency)
 return {paid:paid===null?null:format(paid),currency:paid===null?'':charge.card_currency,authorization:charge?.type==='1',posted:charge?.status==='2',returned,pending,invalid,
  difference:comparable?format(paid-(totals.get(charge.card_currency)||0n)):null,
  reason:paid===null?'付款流水待关联或金额缺失':charge.status!=='2'?'付款流水待入账':invalid?'回款金额待核实':!comparable?'币种不同，不能直接相减':''}
}
