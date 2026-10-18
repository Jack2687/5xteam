import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { webcrypto } from 'node:crypto'
import { ref, reactive, computed, watch } from 'vue'
import * as helpers from '../src/nexovcc.js'
import * as finance from '../src/nexovcc-finance.js'

const code=readFileSync(new URL('../src/components/NexovccManagementView.vue',import.meta.url),'utf8').split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm,'')
for(const scenario of ['success','processing','unknown','occupied','cancel balance']){
 const requests=[]
 const rows=[10,20,1].map((balance,i)=>({card:{id:'card'+i,serial_no:'serial'+i,last4:'000'+i,status:'1',balance:String(balance),imported:true},local:{active_accounts:scenario==='occupied'&&i===0?1:0,pending_accounts:0}}))
 const context=vm.createContext({...helpers,...finance,ref,reactive,computed,watch,crypto:webcrypto,
  defineProps:()=>({defaultPageSize:10}),defineEmits:()=>()=>{},onMounted(){},onBeforeUnmount(){},clearTimeout(){},setTimeout(){},formatTime:v=>v,
  async api(url,options={}){
   if(url==='/api/nexovcc/dashboard')return {cards:rows,cases:[],operations:[],products:[],balance:{},groups:[],finance:{}}
   if(url==='/api/nexovcc/job')return {status:'completed'}
   if(url!=='/api/nexovcc/run')throw Error(url)
   const body=JSON.parse(JSON.stringify(options.body));requests.push(body)
   if(body.action==='refresh')return {status:'completed'}
   return {status:scenario==='processing'||scenario==='unknown'?'attention':'completed',operation:{status:scenario==='unknown'?'unknown':scenario==='processing'?'processing':'succeeded'}}
  }
 })
 vm.runInContext(code+';globalThis.view={data,batchCards,batch,message};',context)
 const view=context.view;view.data.cards=rows
 await view.batchCards({confirm:true,ids:rows.map(r=>r.card.id),action:scenario==='cancel balance'?'cancel':'withdraw'})
 const sent=requests.filter(r=>r.action!=='refresh')
 if(scenario==='success'||scenario==='processing'){
  assert.deepEqual(sent.map(r=>r.amount),['9.00','19.00'])
  assert.equal(new Set(sent.map(r=>r.request_id)).size,2)
  assert.equal(view.batch.value.done,3)
  assert.equal(view.batch.value.skipped,1)
  assert.equal(view.batch.value.pending,scenario==='processing'?2:0)
 }else if(scenario==='unknown')assert.equal(sent.length,1)
 else assert.equal(sent.length,0)
 assert.equal(view.batch.value.running,false)
}
console.log('Passed: batch withdraw keeps 1 USD, skips low balances, continues acknowledged pending cards, stops unknown results, blocks occupied cards and unwithdrawn cancellation')
