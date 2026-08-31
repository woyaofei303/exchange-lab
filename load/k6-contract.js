/** Bounded HTTP contract load scenario; results are local regression evidence only. */
import http from 'k6/http';
import ws from 'k6/ws';
import { check, sleep } from 'k6';
export const options={vus:10,duration:'30s',thresholds:{http_req_duration:['p(95)<200'],http_req_failed:['rate<0.01']}};
const base=__ENV.BASE_URL||'http://127.0.0.1:18080';
export default function(){
  const ticker=http.get(`${base}/v1/tickers?symbol=BTCUSDT-PERP`);check(ticker,{'ticker 200':r=>r.status===200});
  const prices=http.get(`${base}/v1/prices?symbol=BTCUSDT-PERP`);check(prices,{'risk price 200':r=>r.status===200});
  if(__ENV.WS_URL){ws.connect(__ENV.WS_URL,{},socket=>{socket.setTimeout(()=>socket.close(),500)})}
  sleep(0.2);
}
