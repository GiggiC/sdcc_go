function getPositionAtLeastOnce(position) {

    var title = $('#title').val();
    var message = $('#message').val();
    var topic = $('#topic').val();
    var radius = $('#radius').val();
    var lifeTime = $('#lifeTime').val();
    var latitude = position.coords.latitude;
    var longitude = position.coords.longitude;

    $.ajax({
        type: "POST",
        url: "/publish",
        timeout: $('#deliveryTimeout'),
        data: JSON.stringify({
            Message: message, Topic: topic, Title: title, Radius: radius, LifeTime: lifeTime,
            Latitude: latitude, Longitude: longitude
        }),
        success: function (data) {
            if (data === "fail") {
                console.log(this.timeout)

                $.ajax(this);
            } else {
                alert("Message Published!");
                window.location.href = '/publishPage'
            }
        },
        error: function (jqXHR, textStatus) {
            console.log(this.timeout)
            if (textStatus === 'timeout') {
                $.ajax(this);
            }
        }
    })
}

function getPositionAtMostOnce(position) {

    var title = $('#title').val();
    var message = $('#message').val();
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
        timeout: $('#deliveryTimeout'),
        tryCount: 0,
        retryLimit: $('#retryLimit'),
        data: JSON.stringify({
            Message: message, Topic: topic, Title: title, Radius: radius, LifeTime: lifeTime,
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

    var title = $('#title').val();
    var message = $('#message').val();
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
        timeout: $('#deliveryTimeout'),
        data: JSON.stringify({
            Message: message, Topic: topic, Title: title, Radius: radius, LifeTime: lifeTime,
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