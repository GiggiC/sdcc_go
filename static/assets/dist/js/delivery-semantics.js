function getPositionAtLeastOnce(position) {

    var payload = $('#payload').val();
    var topic = $('#topic').val();
    var radius = $('#radius').val();
    var lifeTime = $('#lifeTime').val();
    var latitude = position.coords.latitude;
    var longitude = position.coords.longitude;

    $.ajax({
        type: "POST",
        url: "/publish",
        timeout: 3000,
        data: JSON.stringify({
            Message: payload, Topic: topic, Radius: radius, LifeTime: lifeTime,
            Latitude: latitude, Longitude: longitude
        }),
        success: function (data) {
            if (data === "fail") {
                $.ajax(this);
            } else {
                alert("Message Published!");
                window.location.href = '/publishPage'
            }
        },
        error: function (jqXHR, textStatus) {
            console.log(textStatus)
            if (textStatus === 'timeout') {
                $.ajax(this);
            }
        }
    })
}

function getPositionAtMostOnce(position) {

    var payload = $('#payload').val();
    var topic = $('#topic').val();
    var radius = $('#radius').val();
    var lifeTime = $('#lifeTime').val();
    var latitude = position.coords.latitude;
    var longitude = position.coords.longitude;

    var date = Date.now();
    var email = $('#email').val();
    var id = email + date

    $.ajax({
        type: "POST",
        url: "/publish",
        timeout: 3000,
        tryCount: 0,
        retryLimit: 5,
        data: JSON.stringify({
            Message: payload, Topic: topic, Radius: radius, LifeTime: lifeTime,
            Latitude: latitude, Longitude: longitude, RequestID: id
        }),
        success: function (data) {
            if (data === "fail") {
                $.ajax(this);
            } else {
                alert("Message Published!");
                window.location.href = '/publishPage'
            }
        },
        error: function (jqXHR, textStatus) {
            if (textStatus === 'timeout') {
                this.tryCount++;
                if (this.tryCount < this.retryLimit) {
                    $.ajax(this);
                }
            }
        }
    })
}

function getPositionExactlyOnce(position) {

    var payload = $('#payload').val();
    var topic = $('#topic').val();
    var radius = $('#radius').val();
    var lifeTime = $('#lifeTime').val();
    var latitude = position.coords.latitude;
    var longitude = position.coords.longitude;

    var date = Date.now();
    var email = $('#email').val();
    var id = email + date

    $.ajax({
        type: "POST",
        url: "/publish",
        timeout: 3000,
        data: JSON.stringify({
            Message: payload, Topic: topic, Radius: radius, LifeTime: lifeTime,
            Latitude: latitude, Longitude: longitude, RequestID: id
        }),
        success: function (data) {
            if (data === "fail") {
                $.ajax(this);
            } else {
                alert("Message Published!");
                $.ajax({
                    type: "POST",
                    url: "/removeRequest",
                    data: JSON.stringify({
                        RequestID: id
                    }),
                })
                window.location.href = '/publishPage'
            }
        },
        error: function (jqXHR, textStatus) {
            if (textStatus === 'timeout') {
                $.ajax(this);
            }
        }
    })
}