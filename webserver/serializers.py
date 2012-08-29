from db.models import Author, Book

def serialize(obj):
    return {
        Author: serialize_author,
        Book: serialize_book,
    }[obj.__class__](obj)


def serialize_book(book):
    return {
        'id': book.id,
        'title': book.title,
        'genre': book.genre.name,
        'year': book.year,
        'author': serialize(book.author)
    }


def serialize_author(author):
    return {
        'first_name': author.first_name,
        'last_name': author.last_name,
        'middle_name': author.middle_name,
    }
