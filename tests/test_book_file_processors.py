# -*- coding: utf-8 -*-
from nose.tools import nottest
import os

import unittest2

from book_file_processors import BookFileProcessorFactory
import settings


class BookFileProcessorTestCase(unittest2.TestCase):
    def setUp(self):
        self.processor = self.get_book_file_processor()
        self.book = self.get_book()

    def get_book(self):
        return self.processor.get_book()

    def get_book_file_processor(self):
        return BookFileProcessorFactory(
            self.get_tested_filename()
        ).get_book_file_processor()

    @nottest
    def get_tested_filename(self):
        raise NotImplementedError


class TestFB2BookFileProcessor(BookFileProcessorTestCase):
    @nottest
    def get_tested_filename(self):
        return os.path.join(settings.TESTS_DATA_ROOT, 'dostoevskii_fedor_besy.fb2')

    def test(self):
        self.assertEqual(u'Федор', self.book.author_first_name)
        self.assertEqual(u'Михайлович', self.book.author_middle_name)
        self.assertEqual(u'Достоевский', self.book.author_last_name)
        self.assertEqual(u'Бесы', self.book.title)
        self.assertEqual(self.get_tested_filename(), self.book.path)


class TestZippedFB2BookFileProcessor(BookFileProcessorTestCase):
    @nottest
    def get_tested_filename(self):
        return os.path.join(settings.TESTS_DATA_ROOT, 'dostoevskii_fedor_besy.fb2.zip')

    def test(self):
        self.assertEqual(u'Федор', self.book.author_first_name)
        self.assertEqual(u'Михайлович', self.book.author_middle_name)
        self.assertEqual(u'Достоевский', self.book.author_last_name)
        self.assertEqual(u'Бесы', self.book.title)
        self.assertEqual(self.get_tested_filename(), self.book.path)


