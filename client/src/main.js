/**
 * Created by luweijia on 2018/12/21.
 */

var ws = new WebSocket("wss://echo.websocket.org");

var local_status = 0;
var local_position = 0.0;
var local_speed = 1.0;
var local_rtt = 0.0;

var playback_status_type = {
    stopped: 0,
    playing: 1,
    paused: 2,
    error: 3
};

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
            var time_info = JSON.parse(rec.payload);
            var send_time = time_info.sendtime;
            var serv_time = time_info.servicetime;

            estimate_latency(send_time, serv_time, rec_time);

            setTimeout(send_ping(), 1000);
            break;
        //get STATE
        case 3:
            var playback_state = JSON.parse(rec.payload);
            var src = playback_state.src;//url?use?
            var playback_status = playback_state.status;
            var playback_position = playback_state.position + local_rtt/2;
            var playback_speed = playback_state.speed;
            if(local_position - playback_position > 0.5 || local_position - playback_position < -0.5){
                local_position = playback_position;
            }
            local_status = playback_status;//is there any bug? confused about STOPED->PAUSED
            local_speed = playback_speed;
            break;
        //get STATEUPDATE
        case 4:
            console.error("On message should not receive STATEUPDATE")
            //not the case, should be send message
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

function sendStateUpdate(){
    //should be used together with webpage
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
}

function estimate_latency(send_t, serve_t, rec_t) {
    var lat = (rec_t - send_t - serve_t) / 2.0;
    console.log("current latency = " + lat);
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
    // lat_index = lat_winsize % (lat_index + 1); // why error??
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
            console.log("Latency: outside");
        }
        estimation = alpha * estimation + (1 - alpha) * lat;
        console.log("Estimation: " + estimation);
        local_rtt = estimation;
    }
}
