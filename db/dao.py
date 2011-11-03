from db.base import Session
from db.models import Author, Book


class BooksDao(object):
    def __init__(self):
        self.session = Session()

    def book_with_path_already_exists(self, path):
        try:
            self.get_book_by_path(path)
            return True
        except Book.NotFound:
            return False

    def get_book_by_path(self, path):
        return self.session.query(Book).filter_by(path=path).one()

    def save_book(self, book_object):
        book_builder = BookBuilder(book_object)
        author = book_builder.create_author().save()
        book = book_builder.create_book()
        book.author = author
        book.save()


class BookBuilder(object):
    def __init__(self, book_object):
        self.book_object = book_object

    def create_author(self):
        return Author(
            first_name=self.book_object.author_first_name,
            middle_name=self.book_object.author_middle_name,
            last_name=self.book_object.author_last_name,
        )

    def create_book(self):
        return Book(path=self.book_object.path, title=self.book_object.title)
