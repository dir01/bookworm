# -*- coding: utf-8 -*-
import os

from nose.tools import assert_equal, nottest

from book_file_processors import BookFileProcessorFactory
import settings


class BookFileProcessorTestCase(object):
    def setup(self):
        self.processor = self.get_book_file_processor()
        self.book = self.get_book()

    def get_book(self):
        return self.processor.get_book()

    def get_book_file_processor(self):
        return BookFileProcessorFactory(
            self.get_tested_filename()
        ).get_book_file_processor()

    def get_tested_filename(self):
        raise NotImplementedError


class TestFB2BookFileProcessor(BookFileProcessorTestCase):
    @nottest
    def get_tested_filename(self):
        return os.path.join(settings.TESTS_DATA_ROOT, 'dostoevskii_fedor_besy.fb2')

    def test(self):
        assert_equal(u'Федор', self.book.author_first_name)
        assert_equal(u'Михайлович', self.book.author_middle_name)
        assert_equal(u'Достоевский', self.book.author_last_name)
        assert_equal(u'Бесы', self.book.title)
        assert_equal(self.get_tested_filename(), self.book.path)


class TestZippedFB2BookFileProcessor(BookFileProcessorTestCase):
    @nottest
    def get_tested_filename(self):
        return os.path.join(settings.TESTS_DATA_ROOT, 'dostoevskii_fedor_besy.fb2.zip')

    def test(self):
        assert_equal(u'Федор', self.book.author_first_name)
        assert_equal(u'Михайлович', self.book.author_middle_name)
        assert_equal(u'Достоевский', self.book.author_last_name)
        assert_equal(u'Бесы', self.book.title)
        assert_equal(self.get_tested_filename(), self.book.path)


