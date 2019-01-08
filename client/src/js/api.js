var ws;
var rid;
var m_token;
var g_token;

const api_url = "http://localhost:8081/";

function create_room() {
    $.ajax({
        url: api_url + "room",
        success: function(rec) {
            get_data(rec);
        },
        error: function() { alert('Server has problem'); }
    });
}

function get_data(rec) {
    if(rec.ok != true) {
        alert('You cannot create a room now');
        return;
    }
    window.location.href="test.html";
    var rid = rec.roomID;
    var m_token = rec.masterToken;
    var g_token = rec.guestToken;
    //var join_url = ws_url + "?rid=" + rid + "&token=" + m_token;
    //ws = new WebSocket(join_url, "vchamber_v1");
    console.log("finish and go to media html");
}
