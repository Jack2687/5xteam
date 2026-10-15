import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { ref, reactive } from 'vue'
import { accountAmounts } from '../src/ejh-account-amounts.js'

const charge={type:'4',status:'2',card_amount:'100.30',card_currency:'USD'}
const returned=(id,amount,currency='USD',extra={})=>({id,confirmed:true,confirmed_amount:amount,confirmed_currency:currency,trade:{tradeType:'5',tradeStatus:'2'},...extra})
const a=returned('a','0.10'),b=returned('b','0.20')
assert.equal(accountAmounts({charge,returns:[a,b,a]}).difference,'100.00')
assert.deepEqual(accountAmounts({charge,returns:[a,b]}).returned,[{amount:'0.30',currency:'USD'}])
assert.equal(accountAmounts({charge,returns:[returned('a','101.40')]}).difference,'-1.10')
assert.equal(accountAmounts({credits:{charged:100},returns:[a]}).paid,null)
assert.equal(accountAmounts({charge:{...charge,card_amount:''}}).difference,null)
assert.equal(accountAmounts({charge:{...charge,card_amount:'0'}}).difference,'0.00')
assert.equal(accountAmounts({charge:{...charge,card_amount:'0.000001'}}).difference,'0.000001')
assert.equal(accountAmounts({charge:{...charge,status:'1'}}).difference,null)
assert.equal(accountAmounts({charge:{...charge,type:'1'}}).authorization,true)
assert.equal(accountAmounts({charge:{...charge,type:'3'}}).paid,null)
assert.equal(accountAmounts({charge,returns:[returned('a','1','EUR')]}).difference,null)
assert.equal(accountAmounts({charge,returns:[returned('a','bad')]}).invalid,true)
assert.equal(accountAmounts({charge,returns:[returned('a','bad')]}).difference,null)
for(const amount of ['-1','1e2','NaN','9999999999999','0.0000001'])assert.equal(accountAmounts({charge:{...charge,card_amount:amount}}).paid,null)
const unconfirmed=accountAmounts({charge,returns:[returned('a','10','USD',{confirmed:false}),returned('b','10','USD',{trade:{tradeType:'2',tradeStatus:'1'}}),returned('c','99','USD',{trade:{tradeType:'17',tradeStatus:'2'}})]})
assert.equal(unconfirmed.pending,2)
assert.equal(unconfirmed.returned.length,0)
assert.equal(unconfirmed.difference,'100.30')

const code=readFileSync(new URL('../src/components/EJHCardTrades.vue',import.meta.url),'utf8').split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm,'')
const calls=[],unmounts=[]
let fail=false,release,hold=false
const context=vm.createContext({ref,reactive,AbortController,defineEmits:()=>()=>{},defineExpose(){},onBeforeUnmount:fn=>unmounts.push(fn),async api(url,options){calls.push({url,options});if(hold)await new Promise(resolve=>{release=resolve});if(fail)throw Error('fixture failure');return {items:[{tradeNo:'saved'}],page:options.body.page,has_more:true,queried_at:'2026-10-15T00:00:00Z'}}})
vm.runInContext(code+';globalThis.f={open,query,dates,rows,page,applied,requestedPage,error,busy};',context)
const f=context.f
await f.open({id:'card/one'})
assert.equal(calls[0].url,'/api/ejh/cards/card%2Fone/trades')
assert.equal(calls[0].options.method,'POST')
const original={...calls[0].options.body}
f.dates.start='2020-01-01'
await f.query(2);assert.equal(calls.at(-1).options.body.start,original.start)
await f.query(1);assert.equal(calls.at(-1).options.body.start,original.start,'previous page must retain applied dates')
await f.query(1,true);assert.equal(calls.at(-1).options.body.start,'2020-01-01')
fail=true;await f.query(2)
assert.equal(f.page.value,1);assert.equal(f.rows.value[0].tradeNo,'saved');assert.equal(f.error.value,'fixture failure')
fail=false;hold=true
const pending=f.query(2),count=calls.length
assert.equal(f.requestedPage.value,2);assert.equal(f.busy.value,true)
await f.query(3);assert.equal(calls.length,count,'duplicate request admitted')
unmounts.forEach(fn=>fn());assert.equal(calls.at(-1).options.signal.aborted,true)
release();await pending;assert.equal(f.page.value,1,'disposed dialog was updated')
console.log('Passed: precise payment/return/difference, missing/pending/mixed currency, duplicate and closure exclusions; per-card query, pagination date retention, error retention, progress, duplicate requests and disposal')
