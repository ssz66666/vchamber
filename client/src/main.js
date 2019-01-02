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
    while(!ws.onopen){
        //wait
        console.log("wait ws open")
    }
    if(ws.onopen){
        ws.send(message)
    }
    console.log('Send message:' + message)

    // console.log('websocket closed')
}

function close_ws(){
    // write_document('input','CLOSE')
    if(!ws.onopen){
        console.log("WS is not open")
    }
    else{
        ws.close()
        console.log('Close Websocket')
    }
}