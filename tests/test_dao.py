import unittest2
from mock import patch
from sqlalchemy.engine import create_engine
from sqlalchemy.orm import scoped_session
from sqlalchemy.orm.session import sessionmaker

from book import Book
from db.meta import Base
from db.dao import BooksDao
from db.models import Book as BookModel


class TestBooksDao(unittest2.TestCase):
    SQLALCHEMY_URL = 'sqlite:///:memory:'

    def setUp(self):
        self.tested_dao = self.get_dao_with_test_session()
        self.sync_db()

    def test_getting_missing_book_raises_exception(self):
        with self.assertRaises(BookModel.NoResultFound):
            self.get_book_by_path('/not/existing/path/')

    def test_getting_existing_book_returns_book(self):
        created_book = self.create_book()
        queried_book = self.get_book_by_path(created_book.path)
        self.assertEqual(created_book, queried_book)

    def test_book_with_path_already_exists_returns_false(self):
        self.assertFalse(
            self.book_with_path_already_exists('/not/existing/path/')
        )

    def test_book_with_path_already_exists_returns_true(self):
        created_book = self.create_book()
        self.assertTrue(
            self.book_with_path_already_exists(created_book.path)
        )

    def get_dao_with_test_session(self):
        return BooksDao(self.SQLALCHEMY_URL)

    def sync_db(self):
        from db.models import Base
        Base.metadata.create_all(self.tested_dao.session.bind)

    def get_book_by_path(self, path):
        return self.tested_dao.get_book_by_path(path)

    def book_with_path_already_exists(self, path):
        return self.tested_dao.is_book_with_path_already_exists(path)

    def create_book(self):
        return self.tested_dao.save_book(self.create_fake_book())

    def create_fake_book(self):
        return Book(
            author_first_name='Franz',
            author_middle_name='?',
            author_last_name='Kafka',
            path='/tmp/book',
            title='The Castle',
            genre='Kafkian',
            date='1000',
            language='en'
        )
