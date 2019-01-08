// Handle join a room
var params = window.location.search;
if(params[0] == "?") {
    localStorage.setItem("join", params);
    window.location.href="test.html";
}

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
    localStorage.setItem("rid",rec.roomID);
    localStorage.setItem("m_token",rec.masterToken);
    localStorage.setItem("g_token",rec.guestToken);
    window.location.href="test.html";
}
