$(function(){
    $('#search-input').suggester({
        url: $('#search-form').attr('action')
    })
});
