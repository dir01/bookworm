# -*- coding: utf-8
from pyramid.httpexceptions import HTTPFound
from pyramid.url import route_url
from pyramid.view import view_config

import settings
from db.meta import get_session
from db.models import Book, Author


@view_config(route_name='index', renderer='index.jinja2')
def index(request):
    return {}


@view_config(route_name='search', renderer='books.jinja2')
def search(request):
    term = request.GET.get('query')
    if not term:
        raise HTTPFound(route_url('index', request))
    session = get_session(settings.SQLALCHEMY_URL)
    chunks = term.split()
    query = Book.title.ilike(term)
    if len(chunks) == 2:
        first_name, last_name = chunks
        query |= (Author.first_name.ilike(first_name) & Author.last_name.ilike(last_name))
    else:
        query |= Author.first_name.ilike(term)
        query |= Author.last_name.ilike(term)

    books = session.query(Book).join(Author).filter(query)
    return {
        'books': books
    }
