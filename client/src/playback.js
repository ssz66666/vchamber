const PING_INTERVAL = 1000 // 1sec

const player = new Plyr('#player');

// Expose
window.player = player;
// player.source = {
//     type: 'video',
//     sources: [
//         {
//             src: 'https://www.youtube.com/watch?v=rKcWT1LIH3M',
//             provider: 'youtube',
//         },
//     ],
// }

// Bind event listener
function on(selector, type, callback) {
    document.querySelector(selector).addEventListener(type, callback, false);
}

// Play
on('.js-play', 'click', () => {
    player.play();
});

// Pause
on('.js-pause', 'click', () => {
    player.pause();
});

// Stop
on('.js-stop', 'click', () => {
    player.stop();
});

// Rewind
on('.js-rewind', 'click', () => {
    player.rewind();
});

// Forward
on('.js-forward', 'click', () => {
    player.forward();
});

//var ws = new WebSocket("wss://echo.websocket.org");

//var ws = new WebSocket("ws://129.213.173.180:8080/ws?rid=testroom&token=iamgod", "vchamber_v1");
var ws = new WebSocket("ws://localhost:8080/ws?rid=testroom&token=iamgod", "vchamber_v1");

//var local_src = '';
var master_client = true;
var load_finished = false;
var src_change = false;
var status_change = false;
var rate_change = false;
// var local_status = 0;
// var local_position = 0.0;
// var local_speed = 1.0;
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
    reserved: 99
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

var clientStatus = ''
var latestStateUpdate = null
var pingTicker

ws.onopen = function(evt) {
    console.log("Connection open ...")

    //send ping message first
    send_ping()
    addPlyrEventHandlers()
    pingTicker = setInterval(send_ping, PING_INTERVAL)
};

ws.onmessage = function(evt) {
    console.log( "Received Message: " + evt.data);

    var rec_time = new Date() / 1000;
    var rec = JSON.parse(evt.data);
    //logic part
    switch(rec.type) {
        //get HELLO
        case 0:
            clientStatus = rec.payload.authority
            break;
        //get PONG
        case 2:
            var time_info = rec.payload;
            var send_time = time_info.sendtime;
            var serv_time = time_info.servicetime;

            estimate_latency(send_time, serv_time, rec_time);

            // setTimeout(send_ping, PING_INTERVAL);
            break;
        //get STATE
        case 3:
            var playback_state = rec.payload
            var _src = playback_state.src;//url?use?
            //src change
            if(_src == '') {
                // invalid source, ignore the remote state
                break
            }
            var src = JSON.parse(decodeURIComponent(_src))

            if(player.source!=src){
                removePlyrEventHandlers()
                player.source = src;
                addPlyrEventHandlers()
            }
            if(player.seeking) {
                // user is seeking right now, don't annoy the user just yet
                latestStateUpdate = playback_state
                break
            }
            updateLocalState(playback_state)

            break;
        default:
            break;
    }
};

// function receiveTest(){
//     var type = 3;
//     var rece = '{"status":"2", "position":100, "speed":2.0, "src":"t.mp4"}'
//     //var rece = '{"type": "3", "payload": "[{"status": "2", "position": "100", "speed":"2.0", "src":"t.mp4"}]"}'
//     var rec_time = new Date() / 1000;
//     var rec = rece;//JSON.parse(rece);
//     //console.log("receive json: " + rec.type + rec.payload);
// //logic part
//     switch(type) {
//         //get HELLO
//         case 0:
//
//             break;
//         //get PONG
//         case 2:
//             var time_info = JSON.parse(rec);
//             var send_time = time_info.sendtime;
//             var serv_time = time_info.servicetime;
//
//             estimate_latency(send_time, serv_time, rec_time);
//
//             setTimeout(send_ping(), 1000);
//             break;
//         //get STATE
//         case 3:
//             var playback_state = JSON.parse(rec);
//             console.log("STATE:"+playback_state);
//             var src = playback_state.src;//url?use?
//             var playback_status = playback_state.status;
//             var playback_position = playback_state.position + local_rtt;
//             var playback_speed = playback_state.speed;
//             if(local_src!=src){
//                 local_src = src;
//                 src_change = true;
//             }
//             else{
//                 src_change = false;
//             }
//             if(local_position - playback_position > 0.5 || local_position - playback_position < -0.5){
//                 local_position = playback_position;
//             }
//             if(local_status != playback_status){
//                 local_status = playback_status;//is there any bug? confused about STOPED->PAUSED
//                 status_change = true;
//             }
//             else{
//                 status_change = false;
//             }
//             if(local_speed != playback_speed){
//                 local_speed = playback_speed;
//                 rate_change = true;
//             }
//             else{
//                 rate_change = false;
//             }
//             stateUpdate();
//             break;
//         //get STATEUPDATE
//         case 4:
//             console.error("On message should not receive STATEUPDATE")
//             //not the case, should be send message
//             break;
//         default:
//             break;
//     }
// }

ws.onclose = function(evt) {
    console.log("Connection closed.");
    removePlyrEventHandlers()
    clearInterval(pingTicker)
    //close alert
};

var updateLocalState = function(newState){
    var playback_state = newState;
            var src = playback_state.src;//url?use?
            var playback_status = playback_state.status;
            var playback_position = playback_state.position + local_rtt;
            var playback_speed = playback_state.speed;
            latestStateUpdate = null

            var tolerance = Math.max((ucl - lcl) / 2, 0.1)
            if((player.currentTime - playback_position > tolerance) || (player.currentTime - playback_position < -tolerance)){
                if(playback_position == 0){
                    console.log("000000 RECEIVE")
                }
                removePlyrEventHandlers()
                player.currentTime = playback_position;
                addPlyrEventHandlers()
            }
            if((playback_status == playback_status_type.stopped) && (!player.stopped) && (!player.ended)){
                //some bug here
                console.log('STOP RECEIVED');
                removePlyrEventHandlers()
                player.stop();
                addPlyrEventHandlers()
            }
            else if((playback_status == playback_status_type.playing) && !player.playing){
                console.log('PLAYING RECEIVED');
                removePlyrEventHandlers()
                var pm = player.play();
                pm.finally(()=> {
                    addPlyrEventHandlers()
                })
            }
            else if((playback_status == playback_status_type.paused) && !player.paused){
                console.log('PAUSE RECEIVED');
                removePlyrEventHandlers()
                player.pause();
                addPlyrEventHandlers()
            }
            if(player.playbackRate != playback_speed){
                removePlyrEventHandlers()
                player.playbackRate = playback_speed;
                addPlyrEventHandlers()
            }
}

var stateChanged = function(evt){
    var msg = stateToJsonString()
    send_message(msg)
}

var seekedAndPaused = function(evt){
    if (latestStateUpdate != null) {
        // last local state update was interrupted
        updateLocalState(latestStateUpdate)
        latestStateUpdate = null
    }
    else if (evt.detail.plyr.paused) {
        stateChanged(evt)
    }
}

var addPlyrEventHandlers = function(){
    player.on('playing', stateChanged)
    player.on('pause', stateChanged)
    player.on('ended', stateChanged)
    player.on('ratechange', stateChanged)
    player.on('seeked', seekedAndPaused)
}

var removePlyrEventHandlers = function(){
    player.off('playing', stateChanged)
    player.off('pause', stateChanged)
    player.off('ended', stateChanged)
    player.off('ratechange', stateChanged)
    player.off('seeked', seekedAndPaused)
}

// //statechange EVENT is only available with youTube
// player.on('pause', function () {
//     console.log('PAUSE DETECTED');
//     if(true){
//         var message = stateToJsonString();
//         send_message(message);
//     }
// });

// player.on('playing', function () {
//     console.log('PLAYING DETECTED');
//     // console.log(player.source)
//     if(true){
//         var message = stateToJsonString();
//         send_message(message);
//     }
// });

// player.on('ended', function () {
//     console.log('ENDED DETECTED');
//     if(true){
//         var message = stateToJsonString();
//         send_message(message);
//     }
// });

// player.on('ratechange', function () {
//     console.log('RATE DETECTED');
//     if(true){
//         var message = stateToJsonString();
//         send_message(message);
//     }
// });

function write_document(id, text) {
    document.getElementById(id).innerHTML = text;
}

function sendStateUpdate(){
    //should be used together with webpage
}

function send_message(message){
    if(master_client == false || player.duration == 0){
        return;
    }
    if(!load_finished && JSON.parse(message).type == msg_type.stateupdate){
        load_finished = true;
        console.log("LOAD FINISHED");
        return;
    }
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
        // console.log('Send message:' + message)
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
    send_message(send_data)

}

// function stateUpdate(){
//     //DEBUG USE
//     console.log(player.media.currentSrc);
//     // console.log(player.media.paused);
//     // console.log(player.media.ended);
//     // console.log(player.media.muted);
//     console.log(player.media.state);
//     //setSrc
//     if(src_change){
//         //player.source.sources.src = local_src;
//         // player.source = {
//         //     type: 'video',
//         //     sources: [{
//         //         src: local_src,
//         //         provider: 'youtube'
//         //     }]
//         // };
//     }
//     //setStatus
//     if(local_status == playback_status_type.stopped && !player.stopped){
//         console.log('STOP ACTION');
//         player.stop();
//     }
//     else if(local_status == playback_status_type.playing && !player.playing){
//         console.log('PLAYING ACTION');
//         player.play();
//     }
//     else if(local_status == playback_status_type.paused && !player.paused){
//         console.log('PAUSE ACTION');
//         player.pause();
//     }
//
//     //setPosition
//     player.media.currentTime = local_position;
//
//     //setSpeed
//     if(player.media.playbackRate != local_speed) {
//         player.media.playbackRate = local_speed;
//     }
//
// }

function stateToJsonString(){
    var temp_status;
    if(player.paused){
        temp_status = playback_status_type.paused;
    }
    else if(player.stopped || player.ended){
        temp_status = playback_status_type.stopped;
    }
    else if(player.playing){
        temp_status = playback_status_type.playing;
    }
    var payload =
        {
            // TODO: Ask hyun what local_rtt is? is it the RTT(round-trip-time) or the latency?
            "rtt": local_rtt * 2.0, 
            "state": {
                "src":encodeURIComponent(JSON.stringify(player.source)), //source is a string, not a JSON object
                "status":temp_status,
                "position":player.currentTime,
                "speed":player.playbackRate,
                "duration":player.duration
            }
        };
    var updateMsg = {
        "type": msg_type.stateupdate,
        "payload": payload
    }
    // console.log("JSON"+player.media.currentTime);
    return JSON.stringify(updateMsg);
}

function average(data){
    var sum = data.reduce(function(sum, value){
        return sum + value;
    }, 0);

    var avg = sum / data.length;
    return avg;
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
