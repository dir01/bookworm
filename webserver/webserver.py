from wsgiref.simple_server import make_server

from pyramid.config import Configurator


def make_app():
    config = Configurator()
    config.include('pyramid_jinja2')
    config.add_jinja2_search_path('webserver:templates')
    config.scan('webserver.views')
    config.add_static_view(name='css', path='webserver:static/css')
    config.add_static_view(name='js', path='webserver:static/js')
    config.add_static_view(name='img', path='webserver:static/img')
    config.add_route('index', '/')
    config.add_route('search', '/search/')

    app = config.make_wsgi_app()
    return app

def serve(host, port):
    app = make_app()
    server = make_server(host, port, app)
    server.serve_forever()
