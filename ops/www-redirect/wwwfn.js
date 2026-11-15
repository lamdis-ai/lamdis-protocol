function handler(event) {
  var req = event.request;
  var host = req.headers.host && req.headers.host.value;
  if (host === 'www.lamdis.ai') {
    var qs = [];
    for (var k in req.querystring) {
      var q = req.querystring[k];
      if (q.multiValue) { q.multiValue.forEach(function (m) { qs.push(k + '=' + m.value); }); }
      else { qs.push(k + (q.value !== '' ? '=' + q.value : '')); }
    }
    return { statusCode: 301, statusDescription: 'Moved Permanently',
      headers: { location: { value: 'https://lamdis.ai' + req.uri + (qs.length ? '?' + qs.join('&') : '') }, 'cache-control': { value: 'max-age=86400' } } };
  }
  return req;
}
