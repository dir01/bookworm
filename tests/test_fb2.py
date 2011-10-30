# -*- coding: utf8 -*-
import os

from nose.tools import assert_equal, raises

import settings
from fb2_metadata_collector import FB2MetadataCollector, InvalidFB2


class FB2MetadataCollectorTestCase(object):
    def get_collector(self, filename):
        return FB2MetadataCollector.create_instance_by_filename(filename)


class TestRegular(FB2MetadataCollectorTestCase):
    def setup(self):
        self.collector = self.get_collector(
            self.get_correct_book_filename()
        )

    def get_correct_book_filename(self):
        return os.path.join(
            settings.TESTS_DATA_ROOT,
            'dostoevskii_fedor_besy.fb2'
        )

    def test_author_first_name(self):
        assert_equal(u'Федор', self.collector.get_author_first_name())

    def test_author(self):
        assert_equal(u'Михайлович', self.collector.get_author_middle_name())

    def test_author(self):
        assert_equal(u'Достоевский', self.collector.get_author_last_name())

    def test_title(self):
        assert_equal(u'Бесы', self.collector.get_title())

    def test_genre(self):
        assert_equal('prose_rus_classic', self.collector.get_genre())

    def test_date(self):
        assert_equal('1871', self.collector.get_date())

    def test_id(self):
        assert_equal('d85aaac3-2a81-102a-9ae1-2dfe723fe7c7', self.collector.get_id())

    def test_lang(self):
        assert_equal('ru', self.collector.get_lang())


class TestWrongFile(FB2MetadataCollectorTestCase):
    def setup(self):
        self.collector = self.get_collector(self.get_wrong_book_filename())

    def get_wrong_book_filename(self):
        return os.path.join(
            settings.TESTS_DATA_ROOT,
            'dostoevskii_fedor_besy.fb2.zip'
        )

    @raises(InvalidFB2)
    def test_wrong_file(self):
        self.collector.get_title()
