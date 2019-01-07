/**
 * Created by luweijia on 2018/12/21.
 */

var ws = new WebSocket("wss://echo.websocket.org");

var msg_type = {
    hello: 0,
    ping: 1,
    pong: 2,
    state: 3,
    stateupdate: 4,
    reserved: 5
};

// Variables for estimating latency
var lat_winsize = 20;
var cur_index = 0;
var bef_index = -1;
var mov_index = 0;

var estimation = 0;
var ucl = 0, lcl = 0;
var moving_range = 0;

var latencies = new Array();
var movings = new Array();

ws.onopen = function(evt) {
    console.log("Connection open ...");

    //send ping message first
    send_ping();
};

ws.onmessage = function(evt) {
    console.log( "Received Message: " + evt.data);

    var rec_time = new Date() / 1000;
    var rec = JSON.parse(evt.data);
    //logic part
    switch(rec.type) {
        //get HELLO
        case 0:

            break;
        //get PONG
        case 2:
            var time_info = rec.payload;
            var send_time = time_info.sendtime;
            var serv_time = time_info.servicetime;

            estimate_latency(send_time, serv_time, rec_time);

            send_ping();
            break;
        default:
            break;
    }
};

ws.onclose = function(evt) {
    console.log("Connection closed.");
    //close alert
};

function write_document(id, text) {
    document.getElementById(id).innerHTML = text;
}

function send_message(message){
    // write_document('input','SEND')
    var wait_times = 0
    while(ws.readyState != 1){
        //wait
        wait_times++
        console.log("wait ws open")
        //loopout after waiting for a certain period
        //sleep function?
        if(wait_times%100 == 0){
            break;
        }
    }
    if(ws.readyState == 1){
        ws.send(message)
        console.log('Send message:' + message)
    }

    // console.log('websocket closed')
}

function close_ws(){
    // write_document('input','CLOSE')
    // console.log(ws.readyState)
    if(ws.readyState == 0){
        console.log("WS has not been established yet")
    }
    else if(ws.readyState == 2){
        console.log("WS is closing")
    }
    else if(ws.readyState == 3){
        console.log("WS has already been closed")
    }
    else{ //readyState == 1 which means OPEN
        ws.close()
        console.log('Close Websocket')
    }
}

function send_ping() {
    var send_time = new Date() / 1000;
    var payload = '{"sendtime":' + send_time + '}';
    var send_data = '{"type":' + msg_type.ping + ', "payload":' + payload + '}';
    setTimeout(function() {ws.send(send_data);}, 1000);
}

function average(data){
    var sum = data.reduce(function(sum, value){
        return sum + value;
    }, 0);

    var avg = sum / data.length;
    return avg;
}

function estimate_latency(send_t, serve_t, rec_t) {
    serve_t = Number((serve_t).toFixed(3));
    var lat = (rec_t - send_t - serve_t) / 2.0;
    // console.log("current latency = " + lat);

    latencies[cur_index] = lat;
    if(bef_index >= 0) {
        if(movings.length > 0)
            old_moving = movings[mov_index - 1];
        movings[mov_index] = Math.abs(latencies[cur_index] - latencies[bef_index]);
        mov_index++;
        if(mov_index >= lat_winsize) mov_index = 0;
    }
    cur_index++;
    if(cur_index >= lat_winsize) cur_index = 0;

    bef_index++;
    if(bef_index >= lat_winsize) bef_index = 0;

    // Estimate Latency
    if(latencies.length > 10) {
        var sample_mean = average(latencies);
        var moving_mean = average(movings);
        var alpha = 0.1; // Agile filter = indside

        // Update ucl and lcl for inside case
        ucl = sample_mean + 3 * moving_mean / 1.128;
        lcl = sample_mean - 3 * moving_mean / 1.128;

        if(lat > ucl && lat < lcl) {
            // Stable filter = Outside
            // Roll back the movings
            alpha = 0.9;
            mov_index--;
            movings[mov_index] = old_moving;
            moving_mean = average(movings);
            // Update ucl and lcl for outside case
            ucl = sample_mean + 3 * moving_mean / 1.128;
            lcl = sample_mean - 3 * moving_mean / 1.128;
            // console.log("Latency: outside");
        }
        estimation = alpha * estimation + (1 - alpha) * lat;
        // console.log("Estimation: " + estimation);
    }
}
