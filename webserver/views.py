# -*- coding: utf-8
from pyramid.view import view_config

import settings
from db.meta import get_session
from db.models import Book, Author
from .serializers import serialize

session = get_session(settings.SQLALCHEMY_URL)


@view_config(route_name='index', renderer='index.jinja2')
def index(request):
    return {}


class SearchView(object):
    def __init__(self, request):
        self.request = request

    @view_config(route_name='search', renderer='books.jinja2')
    def search_html(self):
        return self.search_books()

    @view_config(route_name='search', renderer='json')
    def search_json(self):
        return self.serialize_books(self.search_books())

    def search_books(self):
        term = self.get_search_term()
        query = Book.title.ilike(term)
        chunks = term.split()
        if len(chunks) == 2:
            first_name, last_name = chunks
            query |= (Author.first_name.ilike(first_name) & Author.last_name.ilike(
                last_name))
        else:
            query |= Author.first_name.ilike(term)
            query |= Author.last_name.ilike(term)
        books = session.query(Book).join(Author).filter(query)
        return books

    def serialize_books(self, books):
        return map(serialize, books) 

    def get_search_term(self):
        return self.request.GET.values()[0]
