import unittest2
from mock import patch
from sqlalchemy.engine import create_engine
from sqlalchemy.orm import scoped_session
from sqlalchemy.orm.session import sessionmaker

from book import Book
from db.base import BaseModel
from db.dao import BooksDao
from db.models import Book as BookModel


class TestBooksDao(unittest2.TestCase):

    def setUp(self):
        engine = create_engine('sqlite:///:memory:', echo=False)
        BaseModel.metadata.create_all(engine)
        self.Session = scoped_session(sessionmaker(bind=engine))
        self.tested_dao = self.get_patched_dao()

    def test_getting_missing_book_raises_not_found(self):
        with self.assertRaises(BookModel.NotFound):
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

    def get_patched_dao(self):
        with self.replace_real_session_with_fake_one('db.dao.Session'):
            return BooksDao()

    def get_book_by_path(self, path):
        return self.tested_dao.get_book_by_path(path)

    def book_with_path_already_exists(self, path):
        return self.tested_dao.book_with_path_already_exists(path)

    def create_book(self):
        with self.replace_real_session_with_fake_one('db.base.Session'):
            return self.tested_dao.save_book(self.create_fake_book())

    def create_fake_book(self):
        return Book(
            author_first_name='Franz', author_middle_name='?', author_last_name='Kafka',
            path='/tmp/book', title='The Castle'
        )

    def replace_real_session_with_fake_one(self, object_string):
        return patch(object_string, new=self.Session)
