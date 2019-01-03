/**
 * Created by luweijia on 2018/12/21.
 */

var ws = new WebSocket("wss://echo.websocket.org");

ws.onopen = function(evt) {
    console.log("Connection open ...");
};

ws.onmessage = function(evt) {
    console.log( "Received Message: " + evt.data);
    //logic part
    // switch(evt.data.type){
    //
    // }
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