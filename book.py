from collections import namedtuple

book_attrs = (
    'path',
    'author_first_name',
    'author_middle_name',
    'author_last_name',
    'title',
    'genre',
    'date',
    'language'
)

Book = namedtuple('Book', ' '.join(book_attrs))