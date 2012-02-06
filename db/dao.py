from db.meta import Base, get_session
from db.models import Author, Book, Genre


class BooksDao(object):
    def __init__(self, url):
        self.session = get_session(url)

    @classmethod
    def get_instance(cls):
        from settings import SQLALCHEMY_URL
        instance = cls(SQLALCHEMY_URL)
        return instance

    def is_book_with_path_already_exists(self, path):
        return bool(self.session.query(Book).filter(Book.path == path).count())

    def get_book_by_path(self, path):
        return self.session.query(Book).filter(Book.path == path).one()

    def save_book(self, book):
        author, _ = self.get_or_create(Author,
            first_name = book.author_first_name,
            middle_name = book.author_middle_name,
            last_name = book.author_last_name,
        )
        genre, _ = self.get_or_create(Genre, name=book.genre)
        db_book = Book(path=book.path, title=book.title)
        db_book.author = author
        db_book.genre = genre
        self.session.add(db_book)
        self.session.commit()
        return db_book

    def get_or_create(self, Model, **kwargs):
        defaults = kwargs.pop('defaults', {})
        instance = self.session.query(Model).filter_by(**kwargs).first()
        if instance:
            return instance, False
        defaults.update(kwargs)
        instance = Model(**defaults)
        self.session.add(instance)
        return instance, True




class BookBuilder(object):
    def __init__(self, book_object):
        self.book_object = book_object

    def build_author(self):
        return Author(
            first_name=self.book_object.author_first_name,
            middle_name=self.book_object.author_middle_name,
            last_name=self.book_object.author_last_name,
        )

    def build_genre(self):
        return Genre(name=self.book_object.genre)

    def build_book(self):
        return Book(path=self.book_object.path, title=self.book_object.title)
