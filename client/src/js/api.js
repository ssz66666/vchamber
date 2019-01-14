// Handle join a room
var params = window.location.search;
if(params[0] == "?") {
    console.log(params);
    window.location.href="room.html" + params;
}

//const api_url = "http://localhost:8081/";

var api_proto = window.location.protocol + '//'

const api_host_path = "api.vchamber.me/";
var api_url = api_proto + api_host_path

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
    var url_ = "room.html"
    url_ = url_+'?rid='+rec.roomID+'&token='+rec.masterToken + '&m='+rec.masterToken + "&g=" + rec.guestToken;
    window.location.href = url_;
}
