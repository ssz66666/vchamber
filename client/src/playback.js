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

// Websocket Connection Logic
var rid = localStorage.getItem("rid");
var m_token = localStorage.getItem("m_token");
var g_token = localStorage.getItem("g_token");
localStorage.removeItem("rid");
localStorage.removeItem("m_token");
localStorage.removeItem("g_token");

var host = "localhost";
var ws_port = "8080";
var ws_addr = "ws://" + host + ":" + ws_port + "/ws?rid=" + rid + "&token=" + m_token;

// For masters
if(m_token != null) {
    var room_url = "http://" + host + ":80" + "/?rid=" + rid;
    var m_url = room_url + "&token=" + m_token;
    var g_url = room_url + "&token=" + g_token;
    document.getElementById("tokens").innerHTML = "Master URL: " + m_url + "<br><br> Guest URL: " + g_url;
}

// For a guest
var join = localStorage.getItem("join");
localStorage.removeItem("join");

if(join != null) {
    ws_addr = "ws://" + host + ":" + ws_port + "/ws" + join;
}

var ws = new WebSocket(ws_addr, "vchamber_v1");
//var ws = new WebSocket("wss://echo.websocket.org");

//var ws = new WebSocket("ws://129.213.173.180:8080/ws?rid=testroom&token=iamgod", "vchamber_v1");
// var ws = new WebSocket("ws://localhost:8080/ws?rid=testroom&token=iamgod", "vchamber_v1");

//SET DEFAULT VIDEO
player.source = {
    type: 'video',
    sources: [{
        src: 'https://www.youtube.com/watch?v=AF1E_DxHQ_A',
        provider: 'youtube'
    }]
};
src_youtube = true;

//var local_src = '';
var master_client = false;
var load_finished = false;
var src_change = false;
var status_change = false;
var rate_change = false;
// var local_status = 0;
// var local_position = 0.0;
// var local_speed = 1.0;
var local_lat = 0.0;
var src_youtube = false;

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
var latestStateUpdate
var missedLatestUpdate = false
var pingTicker
var syncSeeking = false
var firstClick = false

var stable_pause = false
var pauseTimer = null
const bouncyPauseThreshold = 10 // 100 ms

ws.onopen = function(evt) {
    console.log("Connection open ...")

    //send ping message first
    send_ping()
    pingTicker = setInterval(send_ping, PING_INTERVAL)
};

ws.onmessage = function(evt) {
    // console.log( "Received Message: " + evt.data);

    var rec_time = new Date() / 1000;
    var rec = JSON.parse(evt.data);
    //logic part
    switch(rec.type) {
        //get HELLO
        case 0:
            clientStatus = rec.payload.authority
            if(clientStatus == 'master'){
                //master client
                master_client = true;
            }
            else{
                //guest client(no control authority)
                console.log(master_client)
                master_client = false;
            }
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
            console.log("State: " + evt.data)
            var playback_state = rec.payload
            var _src = playback_state.src;//url?use?

            latestStateUpdate = playback_state
            //src change
            if(_src == '') {
                // invalid source, ignore the remote state
                removePlyrEventHandlers()
                addPlyrEventHandlers()
                break
            }
            var src = JSON.parse(decodeURIComponent(_src))

            if(player.source!=src){
                removePlyrEventHandlers()
                player.source = src;
                addPlyrEventHandlers()
            }
            if(player.seeking && !stable_pause) {
                // user is seeking right now, don't annoy the user just yet
                // missedLatestUpdate = true
                break
            }
            updateLocalState(playback_state)

            break;
        default:
            break;
    }
};


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
            var playback_position = playback_state.position + local_lat;
            var playback_speed = playback_state.speed;
            missedLatestUpdate = false
            console.log("Buffering: " + player.buffered)
            var tolerance = Math.max(local_lat, 0.1)
            if((player.currentTime - playback_position > tolerance) || (player.currentTime - playback_position < -tolerance)){
                if(playback_position == 0){
                    console.log("000000 RECEIVE")
                }
                //removePlyrEventHandlers()
                syncSeeking = true
                player.currentTime = playback_position;
                // send a lot playing things when buffering is really heavy or lags...
                //addPlyrEventHandlers()
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
                if (pm != undefined) {
                  pm.then(()=> {
                      addPlyrEventHandlers()
                  },
                  (e)=> {
                      // autoplay got rejected
                      firstClick = true
                      addPlyrEventHandlers()
                  })
                } else {
                  addPlyrEventHandlers()
                }
            }
            else if((playback_status == playback_status_type.paused) && !player.paused){
                console.log('PAUSE RECEIVED');
                removePlyrEventHandlers()
                player.pause();
                stable_pause = true
                addPlyrEventHandlers()
            }
            if(player.speed != playback_speed){
                removePlyrEventHandlers()
                player.speed = playback_speed;
                addPlyrEventHandlers()
            }
}

var stateChanged = function(evt){
    // may not work when a user is seeking while pausing, and the server
    // pushes a conflicting pause to the user
    if (syncSeeking) {
        syncSeeking = false
        return
    }
    if (firstClick) {
        // check the latest state update, if already playing we should wait for next update
        firstClick = false
        if (latestStateUpdate.status == playback_status_type.playing) {
            return
        }
    }
    var msg = stateToJsonString()
    send_message(msg)
    console.log(evt.type)
    console.log('proposing state change')
    console.log(msg)
}

var pauseEvtHandler = function(evt){
    // detect stable pause
    pauseTimer = setTimeout(function(){
      stable_pause = true
      stateChanged(evt)
    }, bouncyPauseThreshold)
}

var findBouncyPause = function(evt){
    clearTimeout(pauseTimer)
}

var seekingHandler = function(evt){
    findBouncyPause(evt)
    if (stable_pause) {
        stateChanged(evt)
    }
}

var addPlyrEventHandlers = function(){
    player.on('play', findBouncyPause)
    player.on('playing', function(evt){
      stable_pause = false
      stateChanged(evt)
    })
    player.on('seeking', seekingHandler)
    player.on('pause', pauseEvtHandler)
    player.on('ended', stateChanged)
    player.on('ratechange', stateChanged)
    // player.on('seeked', seekedAndPaused)
}

var removePlyrEventHandlers = function(){
    player.off('play', findBouncyPause)
    player.off('playing', stateChanged)
    player.off('seeking', seekingHandler)
    player.off('pause', pauseEvtHandler)
    player.off('ended', stateChanged)
    player.off('ratechange', stateChanged)
    // player.off('seeked', seekedAndPaused)
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

function newUrl(){
    if(master_client == false){
        return;
    }
    var youtube_url = document.getElementById("youtube_link").value;
    var mp4_url = document.getElementById("mp4_link").value;
//SOME BUG OF MP4 VERSION
    // if(mp4_url != ''){
    //     console.log("MP4")
    //     player.source = {
    //         type: 'video',
    //         sources: [{
    //             src: mp4_url,
    //             tpye:'video/mp4',
    //             size: 576,
    //         }]
    //     };
    //     console.log("MP4")
    //     src_youtube = false;
    // }
    if(youtube_url != ''){
        console.log("YOUTUBE")
        player.source = {
            type: 'video',
            sources: [{
                src: youtube_url,
                provider: 'youtube'
            }]
        };
        src_youtube = true;
    }
    console.log(player.source)
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
            "rtt": local_lat * 2.0,
            "state": {
                "src":encodeURIComponent(JSON.stringify(player.source)), //source is a string, not a JSON object
                "status":temp_status,
                "position":player.currentTime,
                "speed":player.speed,
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
    // lat_index = lat_winsize % (lat_index + 1); // why error??
    bef_index++;
    if(bef_index >= lat_winsize) bef_index = 0;

    // Estimate Latency
    // if(latencies.length > 10) {
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
        // console.log("Estimation: " + estimation);
        local_lat = estimation;
    // }
}
